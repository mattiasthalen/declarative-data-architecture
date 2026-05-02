// prism/internal/cli/das_run.go
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

func addDasRun(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "run [<source>]",
		Short: "Convenience: prism das land then prism das build",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all || len(args) == 0 {
				return runDasRunAll(cmd.Context(), cmd.OutOrStdout(),
					contractsRoot, lakeRoot, pipelinesRoot, warehousePath)
			}
			return runDasRun(cmd.Context(), cmd.OutOrStdout(),
				contractsRoot, lakeRoot, pipelinesRoot, warehousePath, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "run all sources")
	addDas(root).AddCommand(cmd)
}

func runDasRunAll(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot, warehousePath string) error {
	bs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return err
	}
	for _, b := range bs {
		if err := runDasRun(ctx, out, contractsRoot, lakeRoot, pipelinesRoot, warehousePath, b.SourceID); err != nil {
			return err
		}
	}
	return nil
}

func runDasRun(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot, warehousePath, sourceID string) error {
	fmt.Fprintf(out, "==> %s: land\n", sourceID)
	if err := runDasLand(ctx, out, contractsRoot, lakeRoot, pipelinesRoot, sourceID); err != nil {
		return err
	}
	fmt.Fprintf(out, "==> %s: build\n", sourceID)
	e, err := duckdb.Open(warehousePath)
	if err != nil {
		return err
	}
	defer e.Close()
	return RunDasBuild(ctx, e, out, contractsRoot, lakeRoot, sourceID, true)
}
