// prism/internal/cli/das_discover.go
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/discover"
	"github.com/prism-data/prism/internal/naming"
)

func addDasDiscover(root *cobra.Command) {
	var contractsRoot string
	var update bool
	cmd := &cobra.Command{
		Use:   "discover <source>",
		Short: "Generate (or refresh) per-entity contract scaffolds from upstream metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID := args[0]
			if err := naming.ValidateSnakeCaseIdentifier(sourceID); err != nil {
				return err
			}
			srcDir := filepath.Join(contractsRoot, "das", sourceID)
			srcPath := filepath.Join(srcDir, "_source.yml")
			src, err := contracts.LoadSource(srcPath)
			if err != nil {
				return err
			}
			if src.Source.Provider != "odata" {
				return fmt.Errorf("discover not implemented for provider %q (M1 supports: odata)", src.Source.Provider)
			}
			data, err := fetchMetadata(cmd.Context(), src.Source.BaseURL)
			if err != nil {
				return err
			}
			ents, err := discover.ParseODataMetadata(data)
			if err != nil {
				return err
			}
			for _, ent := range ents {
				yaml, err := discover.RenderEntityScaffold(ent)
				if err != nil {
					return err
				}
				dest := filepath.Join(srcDir, naming.ToSnakeCase(ent.Name)+".yml")
				if _, err := os.Stat(dest); err == nil && !update {
					fmt.Fprintf(cmd.OutOrStdout(), "skip (exists): %s\n", dest)
					continue
				}
				if err := os.WriteFile(dest, []byte(yaml), 0o644); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote: %s\n", dest)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().BoolVar(&update, "update", false, "overwrite existing entity files")
	addDas(root).AddCommand(cmd)
}

// dasGroupOnce ensures `prism das` is added exactly once even if multiple
// `addDas*` registrars are called.
var dasGroup *cobra.Command

func addDas(root *cobra.Command) *cobra.Command {
	if dasGroup != nil {
		return dasGroup
	}
	dasGroup = &cobra.Command{
		Use:   "das",
		Short: "DAS layer subcommands (discover/land/build/run)",
	}
	root.AddCommand(dasGroup)
	return dasGroup
}

func fetchMetadata(ctx context.Context, baseURL string) ([]byte, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = filepath.Join(u.Path, "$metadata")
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("Accept", "application/xml")
	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("$metadata: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
