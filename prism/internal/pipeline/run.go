// prism/internal/pipeline/run.go
package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/prism-data/prism/internal/events"
)

// RunRunner invokes `uv run --project <pipelineDir> python -m prism_dlt_runner
// --source <sourceYAML> --entity <entityYAML>... --lake <lakeDir>`. Stdout
// JSONL events are parsed and dispatched to handler. Stderr is forwarded to
// stderrSink (typically os.Stderr). Returns nil iff the subprocess exited 0
// AND no error event was observed.
func RunRunner(
	ctx context.Context,
	uvPath, pipelineDir string,
	sourceYAML string, entityYAMLs []string,
	lakeDir string,
	handler func(events.Event) error,
	stderrSink io.Writer,
) error {
	args := []string{"run", "--project", pipelineDir, "python", "-m", "prism_dlt_runner",
		"--source", sourceYAML, "--lake", lakeDir,
	}
	for _, e := range entityYAMLs {
		args = append(args, "--entity", e)
	}
	cmd := exec.CommandContext(ctx, uvPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = stderrSink
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start runner: %w", err)
	}

	var sawError bool
	parseErr := events.Parse(bufio.NewReader(stdout), func(e events.Event) error {
		if e.Event == "error" {
			sawError = true
		}
		return handler(e)
	})

	waitErr := cmd.Wait()

	abs, _ := filepath.Abs(pipelineDir)
	switch {
	case parseErr != nil:
		return fmt.Errorf("event parse: %w (pipeline %s)", parseErr, abs)
	case waitErr != nil:
		return fmt.Errorf("runner exit: %w (pipeline %s)", waitErr, abs)
	case sawError:
		return fmt.Errorf("runner emitted error event (pipeline %s)", abs)
	}
	return nil
}
