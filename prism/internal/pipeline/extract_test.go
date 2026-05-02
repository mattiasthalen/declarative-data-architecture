// prism/internal/pipeline/extract_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRunner(t *testing.T) {
	dest := t.TempDir()
	dir, err := ExtractRunner(dest)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(dir))

	// Spot-check expected files exist.
	for _, p := range []string{
		"__init__.py",
		"events.py",
		"runner.py",
		"__main__.py",
		"providers/__init__.py",
		"providers/odata.py",
		"pyproject.toml.tmpl",
	} {
		_, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err, p)
	}
}

func TestExtractRunnerIdempotent(t *testing.T) {
	dest := t.TempDir()
	d1, err := ExtractRunner(dest)
	require.NoError(t, err)
	d2, err := ExtractRunner(dest)
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}
