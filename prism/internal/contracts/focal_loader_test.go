package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
)

func TestLoadFocal_HappyPath(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "contracts", "valid", "dab_simple", "customer.yml")
	f, err := contracts.LoadFocal(path)
	require.NoError(t, err)
	require.Equal(t, 1, f.Version)
	require.Equal(t, "CUSTOMER", f.Entity.ID)
	require.Len(t, f.Attributes, 3)
	require.Equal(t, "STRING", f.Attributes[0].Type)
	require.Empty(t, f.Attributes[0].Group)
	require.Empty(t, f.Attributes[1].Type)
	require.Len(t, f.Attributes[1].Group, 2)
	require.Empty(t, f.Attributes[2].Type)
	require.Len(t, f.Attributes[2].Group, 2)
	require.Len(t, f.MappingGroups, 1)
	require.Equal(t, "adventure_works", f.MappingGroups[0].Name)
	require.Len(t, f.MappingGroups[0].Tables, 1)
	require.Equal(t, "current", f.MappingGroups[0].Tables[0].FromOrDefault())
}

func TestLoadAllDab_WalksDirectory(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "contracts", "valid", "dab_simple")
	bs, err := contracts.LoadAllDab(dir)
	require.NoError(t, err)
	require.Len(t, bs, 1)
	require.Equal(t, "customer", bs[0].EntityID)
	require.Equal(t, "CUSTOMER", bs[0].Focal.Entity.ID)
}
