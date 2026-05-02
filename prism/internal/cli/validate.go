// prism/internal/cli/validate.go
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
		Short: "Validate all contracts under contracts/das/ against the embedded schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			dasDir := filepath.Join(contractsRoot, "das")
			bundles, err := contracts.LoadAll(dasDir)
			if err != nil {
				return err
			}
			for _, b := range bundles {
				fmt.Fprintf(cmd.OutOrStdout(), "OK %s (%d entities)\n", b.SourceID, len(b.Entities))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root (containing das/)")
	root.AddCommand(cmd)
}
