package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
)

func TestValidateFocal_HappyPath(t *testing.T) {
	f, err := contracts.LoadFocal(filepath.Join("..", "..", "testdata", "contracts", "valid", "dab", "customer.yml"))
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateFocal(f))
}

func TestValidateFocal_RejectsInvalid(t *testing.T) {
	cases := []struct {
		file string
		want string // substring expected in the error message
	}{
		{"duplicate_attribute_id.yml", "duplicate attribute id"},
		{"group_partial_binding.yml", "partial group binding"},
		{"group_duplicate_type.yml", "duplicate type"},
		{"multi_identifier_unsupported.yml", "allow_multiple_identifiers: true is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			f, err := contracts.LoadFocal(filepath.Join("..", "..", "testdata", "contracts", "invalid", "dab", tc.file))
			require.NoError(t, err) // parse must succeed; validation rejects
			err = contracts.ValidateFocal(f)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
