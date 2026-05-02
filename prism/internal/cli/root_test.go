// prism/internal/cli/root_test.go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootVersion(t *testing.T) {
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf)
	r.SetErr(&buf)
	r.SetArgs([]string{"--version"})
	require.NoError(t, r.Execute())
	assert.True(t, strings.Contains(buf.String(), "prism"), buf.String())
}
