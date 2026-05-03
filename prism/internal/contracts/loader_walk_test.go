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
	// Two sources: adventure_works (odata) and stripe (rest) added in Task 18.
	require.Len(t, bundle, 2, "expect exactly two sources under valid/das/")

	byID := map[string]*SourceBundle{}
	for _, b := range bundle {
		byID[b.SourceID] = b
	}

	aw, ok := byID["adventure_works"]
	require.True(t, ok, "adventure_works source not found")
	require.NotNil(t, aw.Source)
	assert.Equal(t, "odata", aw.Source.Source.Provider)
	// adventure_works has customer + order entities (order added in Task 17).
	require.Len(t, aw.Entities, 2)
	entsByID := map[string]EntityBundle{}
	for _, e := range aw.Entities {
		entsByID[e.EntityID] = e
	}
	cust, ok := entsByID["customer"]
	require.True(t, ok, "customer entity not found")
	assert.Equal(t, "Customer", cust.Entity.Entity.Name)
	_, ok = entsByID["order"]
	require.True(t, ok, "order entity not found")

	stripe, ok := byID["stripe"]
	require.True(t, ok, "stripe source not found")
	require.NotNil(t, stripe.Source)
	assert.Equal(t, "rest", stripe.Source.Source.Provider)
	require.Len(t, stripe.Entities, 1)
	assert.Equal(t, "customer", stripe.Entities[0].EntityID)
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
