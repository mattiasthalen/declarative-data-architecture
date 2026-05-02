package contracts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSource(t *testing.T) {
	src, err := LoadSource(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/_source.yml"))
	require.NoError(t, err)
	assert.Equal(t, 1, src.Version)
	assert.Equal(t, "odata", src.Source.Provider)
	assert.Equal(t, "https://demodata.grapecity.com/adventureworks/odata/v1/", src.Source.BaseURL)
}

func TestLoadEntity(t *testing.T) {
	ent, err := LoadEntity(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/customer.yml"))
	require.NoError(t, err)
	assert.Equal(t, "Customer", ent.Entity.Name)
	require.NotNil(t, ent.Incremental)
	assert.Equal(t, "ModifiedDate", ent.Incremental.Cursor)
	assert.Equal(t, []string{"customer_id"}, ent.Schema.PrimaryKey)
	require.Len(t, ent.Schema.Columns, 3)
	assert.Equal(t, "customer_id", ent.Schema.Columns[0].TargetName)
	assert.Equal(t, "BIGINT", ent.Schema.Columns[0].Type)
	assert.True(t, ent.Schema.Columns[0].Mode == "REQUIRED")
}
