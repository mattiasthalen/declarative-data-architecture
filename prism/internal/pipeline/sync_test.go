// prism/internal/pipeline/sync_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPyproject(t *testing.T) {
	out, err := renderPyproject("adventure_works", []string{"filesystem"}, "/abs/runner")
	require.NoError(t, err)
	assert.Contains(t, out, `name = "prism-pipeline-adventure_works"`)
	assert.Contains(t, out, `"dlt[filesystem]>=1.5"`)
	assert.Contains(t, out, `path = "/abs/runner"`)
}

func TestEnsurePyprojectWritesAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	changed, err := EnsurePyproject(dir, "adventure_works", []string{"filesystem"}, "/abs/runner")
	require.NoError(t, err)
	assert.True(t, changed)
	// second run with same inputs is a no-op
	changed, err = EnsurePyproject(dir, "adventure_works", []string{"filesystem"}, "/abs/runner")
	require.NoError(t, err)
	assert.False(t, changed)
	// different extras → rewritten
	changed, err = EnsurePyproject(dir, "adventure_works", []string{"filesystem", "sql_database"}, "/abs/runner")
	require.NoError(t, err)
	assert.True(t, changed)

	body, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(body), `"dlt[sql_database]>=1.5"`)
}
