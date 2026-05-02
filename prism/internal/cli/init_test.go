// prism/internal/cli/init_test.go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf)
	r.SetErr(&buf)
	r.SetArgs([]string{"init", "--dir", dir})
	require.NoError(t, r.Execute(), buf.String())

	for _, p := range []string{
		"prism.yml",
		".gitignore",
		"contracts/das/.gitkeep",
	} {
		_, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err, p)
	}

	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gi), "_lake/")
	assert.Contains(t, string(gi), "_pipelines/")
	assert.Contains(t, string(gi), "warehouse.duckdb")
}

func TestInitRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prism.yml"), []byte("x"), 0o644))
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf)
	r.SetErr(&buf)
	r.SetArgs([]string{"init", "--dir", dir})
	require.Error(t, r.Execute())
}
