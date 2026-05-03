// prism/internal/cli/doctor.go
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/engine/duckdb"
	"github.com/prism-data/prism/internal/pipeline"
)

func addDoctor(root *cobra.Command) {
	var contractsRoot, warehousePath string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify environment: uv, DuckDB writability, contracts parse",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Checking environment…")

			uvPath, version, err := pipeline.FindUV(context.Background())
			if err != nil {
				fmt.Fprintf(out, "  uv:           ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  uv:           ✓ %s (%s)\n", version, uvPath)

			if err := writableCheck(warehousePath); err != nil {
				fmt.Fprintf(out, "  warehouse:    ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  warehouse:    ✓ %s writable\n", warehousePath)

			dasDir := filepath.Join(contractsRoot, "das")
			bundles, err := contracts.LoadAll(dasDir)
			if err != nil {
				fmt.Fprintf(out, "  contracts:    ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  contracts:    ✓ %d source(s)\n", len(bundles))

			// DAB cross-layer probe: every contract under contracts/dab references a known DAS contract,
			// and (warning, not error) the corresponding DAS staging table exists in DuckDB.
			dabBs, err := contracts.LoadAllDab(filepath.Join(contractsRoot, "dab"))
			if err != nil {
				fmt.Fprintf(out, "  dab:          ✗ %s\n", err)
				return err
			}
			if err := contracts.ValidateCrossLayer(bundles, dabBs); err != nil {
				fmt.Fprintf(out, "  dab:          ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  dab:          ✓ %d focal(s)\n", len(dabBs))

			// Probe DAS staging tables (warn-only).
			eng, openErr := duckdb.Open(warehousePath)
			if openErr == nil {
				defer eng.Close()
				missing := 0
				for _, b := range dabBs {
					for _, mg := range b.Focal.MappingGroups {
						for _, t := range mg.Tables {
							qschema := "das__" + t.Source
							qtable := t.Entity + "__" + t.FromOrDefault()
							rows, err := eng.Query(context.Background(),
								fmt.Sprintf("SELECT 1 FROM information_schema.tables WHERE table_schema='%s' AND table_name='%s' LIMIT 1;", qschema, qtable))
							if err != nil {
								continue
							}
							ok := rows.Next()
							rows.Close()
							if !ok {
								fmt.Fprintf(out, "  warning: focal %s references %s.%s — not present yet (run prism das build first)\n",
									b.EntityID, qschema, qtable)
								missing++
							}
						}
					}
				}
				if missing == 0 {
					fmt.Fprintf(out, "  staging:      ✓ all DAS staging tables present\n")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	root.AddCommand(cmd)
}

func writableCheck(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
