// prism/internal/cli/das_build.go
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/drift"
	"github.com/prism-data/prism/internal/engine"
	"github.com/prism-data/prism/internal/engine/duckdb"
	"github.com/prism-data/prism/internal/naming"
	"github.com/prism-data/prism/internal/types"
)

func addDasBuild(root *cobra.Command) {
	var contractsRoot, lakeRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "build [<source>]",
		Short: "Generate SQL from contracts; create __historized tables and __current views",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer e.Close()
			if all || len(args) == 0 {
				return RunDasBuildAll(cmd.Context(), e, cmd.OutOrStdout(), contractsRoot, lakeRoot)
			}
			return RunDasBuild(cmd.Context(), e, cmd.OutOrStdout(),
				contractsRoot, lakeRoot, args[0], false,
			)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "build all sources")
	addDas(root).AddCommand(cmd)
}

func RunDasBuildAll(ctx context.Context, e *duckdb.Engine, out io.Writer, contractsRoot, lakeRoot string) error {
	dasDir := filepath.Join(contractsRoot, "das")
	bundles, err := contracts.LoadAll(dasDir)
	if err != nil {
		return err
	}
	for _, b := range bundles {
		if err := RunDasBuild(ctx, e, out, contractsRoot, lakeRoot, b.SourceID, true); err != nil {
			return err
		}
	}
	return nil
}

func RunDasBuild(ctx context.Context, e *duckdb.Engine, out io.Writer, contractsRoot, lakeRoot, sourceID string, alreadyLoaded bool) error {
	if err := naming.ValidateSnakeCaseIdentifier(sourceID); err != nil {
		return err
	}
	dasDir := filepath.Join(contractsRoot, "das")
	bundles, err := contracts.LoadAll(dasDir)
	if err != nil {
		return err
	}
	var bundle *contracts.SourceBundle
	for _, b := range bundles {
		if b.SourceID == sourceID {
			bundle = b
			break
		}
	}
	if bundle == nil {
		return fmt.Errorf("source %s: contract directory not found under %s", sourceID, dasDir)
	}
	d := e.Dialect()
	schema := "das__" + sourceID
	if err := e.Exec(ctx, d.CreateSchemaIfNotExists(schema)); err != nil {
		return err
	}
	for _, ent := range bundle.Entities {
		if err := buildOneEntity(ctx, e, schema, sourceID, lakeRoot, ent); err != nil {
			return fmt.Errorf("entity %s: %w", ent.EntityID, err)
		}
		fmt.Fprintf(out, "  built %s.%s__historized + %s__current\n", schema, ent.EntityID, ent.EntityID)
	}
	return nil
}

func buildOneEntity(ctx context.Context, e *duckdb.Engine, schema, sourceID, lakeRoot string, ent contracts.EntityBundle) error {
	d := e.Dialect()
	cols, err := toEngineColumns(ent.Entity.Schema.Columns)
	if err != nil {
		return err
	}
	tabName := ent.EntityID + "__historized"
	viewName := ent.EntityID + "__current"
	upstream := ent.Entity.Entity.Name // e.g. "Customer"
	lakeGlob, err := filepath.Abs(filepath.Join(lakeRoot, "das", sourceID, upstream, "**/*.jsonl.gz"))
	if err != nil {
		return err
	}
	if err := e.Exec(ctx, d.CreateHistorizedTableIfNotExists(engine.HistorizedTableSpec{
		Schema: schema, Name: tabName, Columns: cols,
	})); err != nil {
		return err
	}
	// Append only if there is at least one JSONL file (DuckDB errors if the
	// glob matches nothing). Use a quick filesystem probe.
	if any, _ := filepath.Glob(filepath.Join(lakeRoot, "das", sourceID, upstream, "*.jsonl.gz")); len(any) == 0 {
		// Try recursive (one-level) probe; treat empty as a no-op append.
		if any2, _ := filepath.Glob(filepath.Join(lakeRoot, "das", sourceID, upstream, "*", "*.jsonl.gz")); len(any2) == 0 {
			// no files yet; skip append, still create the view
		}
	}
	if err := e.Exec(ctx, d.AppendIntoHistorized(engine.HistorizedAppendSpec{
		Schema: schema, Name: tabName, LakeGlob: lakeGlob, Compression: "gzip", Columns: cols,
	})); err != nil {
		return err
	}
	if err := e.Exec(ctx, d.CreateOrReplaceCurrentView(engine.CurrentViewSpec{
		Schema: schema, Name: viewName, HistorizedTable: tabName,
		PrimaryKey: ent.Entity.Schema.PrimaryKey,
	})); err != nil {
		return err
	}
	results, err := drift.DetectNullsInRequired(ctx, e, schema, tabName, cols)
	if err != nil {
		return fmt.Errorf("drift detection: %w", err)
	}
	for _, r := range results {
		return fmt.Errorf("DRIFT: required column %q has %d NULLs in %s.%s — check contract source_path", r.Column, r.NullCount, schema, tabName)
	}
	return nil
}

func toEngineColumns(cs []contracts.Column) ([]engine.Column, error) {
	var out []engine.Column
	for _, c := range cs {
		t, err := types.Parse(c.Type)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.TargetName, err)
		}
		out = append(out, engine.Column{
			SourcePath: c.SourcePath,
			TargetName: c.TargetName,
			SQLType:    t.DuckDBType(),
			NotNull:    c.Mode == "REQUIRED",
		})
	}
	return out, nil
}
