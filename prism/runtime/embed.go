// Package runtime exposes the embedded Python dlt runner tree.
// The embed is declared here because go:embed paths must be relative to the
// declaring file; placing this file adjacent to dlt_runner/ avoids the need
// for symlinks or ".." path elements.
package runtime

import "embed"

//go:embed all:dlt_runner
var RunnerFS embed.FS

// RunnerRoot is the top-level directory name inside RunnerFS.
const RunnerRoot = "dlt_runner"
