// prism/internal/cli/run.go
package cli

import "github.com/spf13/cobra"

func addRun(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot, warehousePath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the entire warehouse end-to-end (M1: equivalent to `prism das run --all`)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDasRunAll(cmd.Context(), cmd.OutOrStdout(),
				contractsRoot, lakeRoot, pipelinesRoot, warehousePath)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	root.AddCommand(cmd)
}
