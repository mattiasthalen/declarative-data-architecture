package cli

import (
	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

func addRun(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot, warehousePath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the entire warehouse end-to-end (DAS + DAB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if err := runDasRunAll(cmd.Context(), out, contractsRoot, lakeRoot, pipelinesRoot, warehousePath); err != nil {
				return err
			}
			eng, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer eng.Close()
			return RunDabBuildAll(cmd.Context(), eng, out, contractsRoot)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	root.AddCommand(cmd)
}
