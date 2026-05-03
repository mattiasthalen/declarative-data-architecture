package cli

import (
	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

func addDabRun(root *cobra.Command) {
	var contractsRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "run [<entity>]",
		Short: "Alias for `prism dab build` (M2 has no separate land step for DAB)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer eng.Close()
			if all || len(args) == 0 {
				return RunDabBuildAll(cmd.Context(), eng, cmd.OutOrStdout(), contractsRoot)
			}
			return RunDabBuild(cmd.Context(), eng, cmd.OutOrStdout(), contractsRoot, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "run all focal entities")
	addDab(root).AddCommand(cmd)
}
