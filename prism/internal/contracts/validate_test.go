package contracts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSourceValid(t *testing.T) {
	src, err := LoadSource(filepath.FromSlash("../../testdata/contracts/valid/das/adventure_works/_source.yml"))
	require.NoError(t, err)
	require.NoError(t, ValidateSource(src))
}

func TestValidateSourceMissingProvider(t *testing.T) {
	src, err := LoadSource(filepath.FromSlash("../../testdata/contracts/invalid/das/missing_provider/_source.yml"))
	require.NoError(t, err)
	err = ValidateSource(src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestValidateEntityValid(t *testing.T) {
	ent, err := LoadEntity(filepath.FromSlash("../../testdata/contracts/valid/das/adventure_works/customer.yml"))
	require.NoError(t, err)
	require.NoError(t, ValidateEntity(ent))
}

func TestValidateEntityDuplicateTargetName(t *testing.T) {
	ent, err := LoadEntity(filepath.FromSlash("../../testdata/contracts/invalid/das/duplicate_target_name/customer.yml"))
	require.NoError(t, err)
	err = ValidateEntity(ent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate target_name")
}

func TestValidateEntityPKReferencesUnknownColumn(t *testing.T) {
	ent := &Entity{
		Version: 1,
		Entity:  EntityIdent{Name: "X"},
		Schema: Schema{
			PrimaryKey: []string{"missing"},
			Columns: []Column{{
				SourcePath: "A", TargetName: "a", Type: "STRING", Mode: "REQUIRED",
			}},
		},
	}
	require.Error(t, ValidateEntity(ent))
}

func TestValidateEntityUnknownType(t *testing.T) {
	ent := &Entity{
		Version: 1,
		Entity:  EntityIdent{Name: "X"},
		Schema: Schema{
			PrimaryKey: []string{"a"},
			Columns: []Column{{
				SourcePath: "A", TargetName: "a", Type: "FLOAT64", Mode: "REQUIRED",
			}},
		},
	}
	require.Error(t, ValidateEntity(ent))
}
