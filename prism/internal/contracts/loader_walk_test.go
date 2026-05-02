package contracts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAll(t *testing.T) {
	bundle, err := LoadAll(filepath.FromSlash("../../testdata/contracts/valid/das"))
	require.NoError(t, err)
	require.Len(t, bundle, 1, "expect exactly one source under valid/das/")
	src := bundle[0]
	assert.Equal(t, "adventure_works", src.SourceID)
	require.NotNil(t, src.Source)
	assert.Equal(t, "odata", src.Source.Source.Provider)
	require.Len(t, src.Entities, 1)
	ent := src.Entities[0]
	assert.Equal(t, "customer", ent.EntityID)
	assert.Equal(t, "Customer", ent.Entity.Entity.Name)
}

func TestLoadAllRejectsBadDirName(t *testing.T) {
	// Create a tmp tree with a Bad-Directory name and assert error.
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "BadName")
	require.NoError(t, mkSourceTree(t, bad))
	_, err := LoadAll(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snake_case")
}

func mkSourceTree(t *testing.T, dir string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src := []byte("version: 1\nsource:\n  provider: odata\n  base_url: https://x.example/\n")
	return os.WriteFile(filepath.Join(dir, "_source.yml"), src, 0o644)
}
