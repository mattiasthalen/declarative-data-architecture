// prism/internal/pipeline/sync.go
package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// providerExtras is the static map from contract `provider:` to dlt extras
// names installed in that source's venv. See ADR-002.
var providerExtras = map[string][]string{
	"odata": {"filesystem"},
	// M2/M3:
	// "rest_api":     {"filesystem"},
	// "sql_database": {"filesystem", "sql_database"},
}

// ExtrasFor returns the dlt extras list for the given provider.
func ExtrasFor(provider string) ([]string, error) {
	x, ok := providerExtras[provider]
	if !ok {
		return nil, fmt.Errorf("no extras mapping for provider %q (M1 supports: odata)", provider)
	}
	return x, nil
}

func renderPyproject(sourceID string, extras []string, runnerPath string) (string, error) {
	var deps strings.Builder
	for _, x := range extras {
		fmt.Fprintf(&deps, "    \"dlt[%s]>=1.5\",\n", x)
	}
	tpl := `[project]
name = "prism-pipeline-%s"
version = "0.0.0"
description = "prism-managed dlt pipeline for source %s"
requires-python = ">=3.11"
dependencies = [
%s    "pyyaml>=6.0",
    "prism-dlt-runner",
]

[tool.uv.sources]
prism-dlt-runner = { path = "%s" }
`
	return fmt.Sprintf(tpl, sourceID, sourceID, deps.String(), runnerPath), nil
}

// EnsurePyproject writes pyproject.toml under dir if missing or if its content
// differs from the rendered output. Returns (changed, error).
func EnsurePyproject(dir, sourceID string, extras []string, runnerPath string) (bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	want, err := renderPyproject(sourceID, extras, runnerPath)
	if err != nil {
		return false, err
	}
	path := filepath.Join(dir, "pyproject.toml")
	have, err := os.ReadFile(path)
	if err == nil && string(have) == want {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// UVSync runs `uv sync --project <dir>` and returns its combined output on error.
func UVSync(ctx context.Context, uvPath, projectDir string) error {
	cmd := exec.CommandContext(ctx, uvPath, "sync", "--project", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("uv sync %s: %w\n%s", projectDir, err, string(out))
	}
	return nil
}
