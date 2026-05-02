// prism/internal/pipeline/uv.go
// Package pipeline orchestrates per-source uv venvs and invokes the embedded
// dlt runner. See ADR-001 and ADR-006.
package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const minUVVersion = "0.5.0"

// FindUV locates the `uv` binary on PATH and verifies its version.
func FindUV(ctx context.Context) (path, version string, err error) {
	path, err = exec.LookPath("uv")
	if err != nil {
		return "", "", fmt.Errorf("uv not found on PATH (install via https://docs.astral.sh/uv/): %w", err)
	}
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return path, "", fmt.Errorf("`uv --version` failed: %w", err)
	}
	version, err = parseUVVersion(string(out))
	if err != nil {
		return path, "", err
	}
	if !versionAtLeast(version, minUVVersion) {
		return path, version, fmt.Errorf("uv %s is older than required minimum %s", version, minUVVersion)
	}
	return path, version, nil
}

func parseUVVersion(out string) (string, error) {
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "uv ") {
		return "", fmt.Errorf("unexpected uv version output: %q", out)
	}
	rest := strings.TrimPrefix(out, "uv ")
	v := strings.SplitN(rest, " ", 2)[0]
	return v, nil
}

func versionAtLeast(have, want string) bool {
	hp := splitVersion(have)
	wp := splitVersion(want)
	for i := 0; i < 3; i++ {
		switch {
		case hp[i] > wp[i]:
			return true
		case hp[i] < wp[i]:
			return false
		}
	}
	return true
}

func splitVersion(v string) [3]int {
	var out [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
