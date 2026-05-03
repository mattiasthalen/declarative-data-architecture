package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
)

func addValidate(root *cobra.Command) {
	var contractsRoot string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate all contracts under contracts/das and contracts/dab",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			dasBs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
			if err != nil {
				return err
			}
			for _, b := range dasBs {
				fmt.Fprintf(out, "OK das/%s (%d entities)\n", b.SourceID, len(b.Entities))
			}
			dabBs, err := contracts.LoadAllDab(filepath.Join(contractsRoot, "dab"))
			if err != nil {
				return err
			}
			for _, b := range dabBs {
				if err := contracts.ValidateFocal(b.Focal); err != nil {
					return fmt.Errorf("focal %s: %w", b.EntityID, err)
				}
				fmt.Fprintf(out, "OK dab/%s\n", b.EntityID)
			}
			if err := contracts.ValidateCrossLayer(dasBs, dabBs); err != nil {
				return err
			}
			fmt.Fprintln(out, "OK cross-layer references")
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	root.AddCommand(cmd)
}
