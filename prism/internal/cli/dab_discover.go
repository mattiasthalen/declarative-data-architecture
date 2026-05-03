package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/types"
)

func addDabDiscover(root *cobra.Command) {
	var contractsRoot string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Scaffold contracts/dab/<entity>.yml for each DAS entity not yet covered",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunDabDiscover(cmd.Context(), cmd.OutOrStdout(), contractsRoot)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	addDab(root).AddCommand(cmd)
}

// RunDabDiscover reads contracts/das/* and writes one focal scaffold to
// contracts/dab/<entity>.yml for each DAS entity that doesn't already have one.
func RunDabDiscover(ctx context.Context, out io.Writer, contractsRoot string) error {
	dasBs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return err
	}
	dabDir := filepath.Join(contractsRoot, "dab")
	if err := os.MkdirAll(dabDir, 0o755); err != nil {
		return err
	}
	existing := map[string]bool{}
	if entries, err := os.ReadDir(dabDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
				existing[strings.TrimSuffix(e.Name(), ".yml")] = true
			}
		}
	}
	for _, b := range dasBs {
		for _, ent := range b.Entities {
			if existing[ent.EntityID] {
				continue
			}
			body := scaffoldFocalYAML(b.SourceID, ent.EntityID, ent.Entity)
			path := filepath.Join(dabDir, ent.EntityID+".yml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(out, "scaffolded contracts/dab/%s.yml from das/%s/%s\n",
				ent.EntityID, b.SourceID, ent.EntityID)
		}
	}
	return nil
}

// scaffoldFocalYAML builds a 1:1 DAB scaffold from a DAS entity.
// Type mapping:
//
//	STRING/VARCHAR  -> STRING
//	NUMBER/INT/...  -> NUMBER
//	TIMESTAMP/DATE  -> STRING (with `# TODO:` comment)
//	BOOLEAN         -> STRING (with `# TODO:` comment)
func scaffoldFocalYAML(sourceID, entityID string, das *contracts.Entity) string {
	upperID := strings.ToUpper(entityID)
	var sb strings.Builder
	sb.WriteString("version: 1\n\n")
	fmt.Fprintf(&sb, "entity:\n  id: %s\n  name: %s\n  definition: \"%s\"\n\n", upperID, upperID, das.Entity.Name)
	sb.WriteString("attributes:\n")
	cols := append([]contracts.Column{}, das.Schema.Columns...)
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].TargetName < cols[j].TargetName })
	for _, c := range cols {
		t := scaffoldType(c.Type)
		todo := ""
		if t == "STRING" && (c.Type == "TIMESTAMP" || c.Type == "DATE" || c.Type == "BOOLEAN") {
			todo = "  # TODO: " + c.Type + " — wrap in START_TIMESTAMP/END_TIMESTAMP group or change type"
		}
		fmt.Fprintf(&sb, "  - id: %s\n    definition: \"%s\"\n    type: %s%s\n",
			strings.ToUpper(c.TargetName), c.TargetName, t, todo)
	}
	sb.WriteString("\nmapping_groups:\n")
	fmt.Fprintf(&sb, "  - name: %s\n", sourceID)
	sb.WriteString("    tables:\n")
	fmt.Fprintf(&sb, "      - source: %s\n", sourceID)
	fmt.Fprintf(&sb, "        entity: %s\n", entityID)
	sb.WriteString("        from: current\n")
	sb.WriteString("        primary_keys:\n")
	for _, pk := range das.Schema.PrimaryKey {
		fmt.Fprintf(&sb, "          - %s\n", pk)
	}
	sb.WriteString("        attributes:\n")
	for _, c := range cols {
		fmt.Fprintf(&sb, "          - id: %s\n            transformation_expression: %s\n",
			strings.ToUpper(c.TargetName), c.TargetName)
	}
	return sb.String()
}

func scaffoldType(t string) string {
	parsed, err := types.Parse(t)
	if err != nil {
		return "STRING"
	}
	sql := parsed.DuckDBType()
	switch {
	case strings.HasPrefix(sql, "VARCHAR"), strings.HasPrefix(sql, "TEXT"):
		return "STRING"
	case strings.HasPrefix(sql, "BIGINT"), strings.HasPrefix(sql, "INTEGER"),
		strings.HasPrefix(sql, "DOUBLE"), strings.HasPrefix(sql, "DECIMAL"):
		return "NUMBER"
	case strings.HasPrefix(sql, "TIMESTAMP"), strings.HasPrefix(sql, "DATE"):
		return "STRING"
	case strings.HasPrefix(sql, "BOOLEAN"):
		return "STRING"
	}
	return "STRING"
}
