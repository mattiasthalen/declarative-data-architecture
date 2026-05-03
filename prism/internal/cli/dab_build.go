package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/dab"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// addDab attaches and returns (idempotently) the `prism dab` parent command.
func addDab(root *cobra.Command) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Use == "dab" {
			return c
		}
	}
	d := &cobra.Command{Use: "dab", Short: "DAB layer commands (focal entities, descriptors, relationships)"}
	root.AddCommand(d)
	return d
}

func addDabBuild(root *cobra.Command) {
	var contractsRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "build [<entity>]",
		Short: "Generate SQL from contracts/dab/; populate dab.* tables and views",
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
	cmd.Flags().BoolVar(&all, "all", false, "build all focal entities")
	addDab(root).AddCommand(cmd)
}

func RunDabBuildAll(ctx context.Context, eng *duckdb.Engine, out io.Writer, contractsRoot string) error {
	dabBs, err := loadAndValidateDab(contractsRoot)
	if err != nil {
		return err
	}
	for _, b := range dabBs {
		if err := executeFocal(ctx, eng, out, b); err != nil {
			return err
		}
	}
	return nil
}

func RunDabBuild(ctx context.Context, eng *duckdb.Engine, out io.Writer, contractsRoot, entityID string) error {
	dabBs, err := loadAndValidateDab(contractsRoot)
	if err != nil {
		return err
	}
	for _, b := range dabBs {
		if b.EntityID == entityID {
			return executeFocal(ctx, eng, out, b)
		}
	}
	return fmt.Errorf("focal entity %q not found under %s", entityID, filepath.Join(contractsRoot, "dab"))
}

func loadAndValidateDab(contractsRoot string) ([]*contracts.FocalBundle, error) {
	dasBs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return nil, err
	}
	dabBs, err := contracts.LoadAllDab(filepath.Join(contractsRoot, "dab"))
	if err != nil {
		return nil, err
	}
	for _, b := range dabBs {
		if err := contracts.ValidateFocal(b.Focal); err != nil {
			return nil, fmt.Errorf("focal %s: %w", b.EntityID, err)
		}
	}
	if err := contracts.ValidateCrossLayer(dasBs, dabBs); err != nil {
		return nil, err
	}
	return dabBs, nil
}

func executeFocal(ctx context.Context, eng *duckdb.Engine, out io.Writer, b *contracts.FocalBundle) error {
	plan, err := dab.BuildEntityPlan(b)
	if err != nil {
		return fmt.Errorf("focal %s: %w", b.EntityID, err)
	}
	if err := dab.Execute(ctx, eng, plan); err != nil {
		return fmt.Errorf("focal %s: %w", b.EntityID, err)
	}
	fmt.Fprintf(out, "  built dab.%s + dab.%s__idfr + dab.%s__descriptor (+ %d rel) + views\n",
		b.EntityID, b.EntityID, b.EntityID, len(plan.Relationships))
	return nil
}
