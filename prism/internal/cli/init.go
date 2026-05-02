// prism/internal/cli/init.go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// addInit attaches the `init` subcommand to the root. Called from NewRoot.
func addInit(root *cobra.Command) {
	var dir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new prism warehouse repo (prism.yml, contracts/, .gitignore)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(dir)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "directory to initialize")
	root.AddCommand(cmd)
}

func runInit(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := refuseIfHasFile(abs, "prism.yml"); err != nil {
		return err
	}
	files := map[string]string{
		"prism.yml": `version: 1
warehouse:
  duckdb_path: ./warehouse.duckdb
paths:
  contracts: ./contracts
  lake:      ./_lake
  pipelines: ./_pipelines
`,
		".gitignore": `# prism warehouse
_lake/
_pipelines/
warehouse.duckdb
*.duckdb.wal
.cache/
`,
		"contracts/das/.gitkeep": "",
	}
	for rel, body := range files {
		full := filepath.Join(abs, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func refuseIfHasFile(dir, name string) error {
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return fmt.Errorf("%s already exists in %s; refusing to overwrite", name, dir)
	}
	return nil
}
