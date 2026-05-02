// prism/internal/pipeline/extract.go
package pipeline

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/prism-data/prism/runtime"
)

// embedRoot is the directory inside the runner FS that contains the runner package.
const embedRoot = runtime.RunnerRoot

// runnerVersion bumps when the embedded runner is changed; used in the cache path.
const runnerVersion = "0.1.0"

// ExtractRunner copies the embedded runner tree into <cacheDir>/<runnerVersion>/dlt_runner/.
// Returns the absolute path of the package directory (suitable for use as a uv path source).
// Re-extraction is a no-op if the destination already exists with the right marker.
func ExtractRunner(cacheDir string) (string, error) {
	root := filepath.Join(cacheDir, runnerVersion, "dlt_runner")
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	marker := filepath.Join(abs, ".prism_runner_version")
	if data, err := os.ReadFile(marker); err == nil && string(data) == runnerVersion {
		return abs, nil
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	err = fs.WalkDir(runtime.RunnerFS, embedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(embedRoot, p)
		if rel == "." {
			return nil
		}
		out := filepath.Join(abs, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := runtime.RunnerFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("extract runner: %w", err)
	}
	if err := os.WriteFile(marker, []byte(runnerVersion), 0o644); err != nil {
		return "", err
	}
	return abs, nil
}
