// prism/internal/cli/doctor.go
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
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
