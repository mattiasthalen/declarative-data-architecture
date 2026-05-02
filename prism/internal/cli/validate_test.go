// prism/internal/cli/validate_test.go
package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateValid(t *testing.T) {
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf)
	r.SetErr(&buf)
	r.SetArgs([]string{"validate", "--contracts", filepath.FromSlash("../../testdata/contracts/valid")})
	require.NoError(t, r.Execute(), buf.String())
	assert.Contains(t, buf.String(), "OK")
}

func TestValidateInvalid(t *testing.T) {
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf)
	r.SetErr(&buf)
	r.SetArgs([]string{"validate", "--contracts", filepath.FromSlash("../../testdata/contracts/invalid")})
	err := r.Execute()
	require.Error(t, err)
}
