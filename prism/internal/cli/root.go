// prism/internal/cli/root.go
// Package cli wires the cobra command tree.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/version"
)

// NewRoot returns the top-level cobra command. Sub-commands are added via
// init() functions in their own files.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "prism",
		Short:         "Declarative data architecture CLI",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return root
}
