// prism/internal/cli/das_land.go
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/events"
	"github.com/prism-data/prism/internal/naming"
	"github.com/prism-data/prism/internal/pipeline"
)

func addDasLand(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot string
	var all bool
	cmd := &cobra.Command{
		Use:   "land [<source>]",
		Short: "Run dlt to land raw JSONL into _lake/das/<source>/<entity>/",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all || len(args) == 0 {
				return runDasLandAll(cmd.Context(), cmd.OutOrStdout(),
					contractsRoot, lakeRoot, pipelinesRoot)
			}
			return runDasLand(cmd.Context(), cmd.OutOrStdout(),
				contractsRoot, lakeRoot, pipelinesRoot, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().BoolVar(&all, "all", false, "land all sources")
	addDas(root).AddCommand(cmd)
}

func runDasLandAll(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot string) error {
	bundles, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return err
	}
	for _, b := range bundles {
		if err := runDasLand(ctx, out, contractsRoot, lakeRoot, pipelinesRoot, b.SourceID); err != nil {
			return err
		}
	}
	return nil
}

func runDasLand(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot, sourceID string) error {
	if err := naming.ValidateSnakeCaseIdentifier(sourceID); err != nil {
		return err
	}
	uvPath, _, err := pipeline.FindUV(ctx)
	if err != nil {
		return err
	}
	bundle, err := loadOneBundle(contractsRoot, sourceID)
	if err != nil {
		return err
	}
	if len(bundle.Entities) == 0 {
		fmt.Fprintf(out, "no entity contracts in %s; nothing to land\n", bundle.SourceDir)
		return nil
	}
	cacheDir := defaultCacheDir()
	runnerPath, err := pipeline.ExtractRunner(cacheDir)
	if err != nil {
		return err
	}
	extras, err := pipeline.ExtrasFor(bundle.Source.Source.Provider)
	if err != nil {
		return err
	}
	pipelineDir, err := filepath.Abs(filepath.Join(pipelinesRoot, sourceID))
	if err != nil {
		return err
	}
	if _, err := pipeline.EnsurePyproject(pipelineDir, sourceID, extras, runnerPath); err != nil {
		return err
	}
	if err := pipeline.UVSync(ctx, uvPath, pipelineDir); err != nil {
		return err
	}
	srcYAML := filepath.Join(bundle.SourceDir, "_source.yml")
	var entYAMLs []string
	for _, ent := range bundle.Entities {
		entYAMLs = append(entYAMLs, ent.Path)
	}
	lakeAbs, _ := filepath.Abs(lakeRoot)
	return pipeline.RunRunner(ctx, uvPath, pipelineDir, srcYAML, entYAMLs, lakeAbs,
		func(e events.Event) error {
			fmt.Fprintf(out, "[%s] %+v\n", e.Event, e)
			return nil
		},
		os.Stderr,
	)
}

func loadOneBundle(contractsRoot, sourceID string) (*contracts.SourceBundle, error) {
	bs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		if b.SourceID == sourceID {
			return b, nil
		}
	}
	return nil, fmt.Errorf("source %s: not found", sourceID)
}

func defaultCacheDir() string {
	if cd, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cd, "prism")
	}
	return filepath.Join(os.TempDir(), "prism-cache")
}
