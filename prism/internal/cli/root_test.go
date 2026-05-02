// prism/internal/cli/root_test.go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestNewRootIsIndependent(t *testing.T) {
	r1 := NewRoot()
	r2 := NewRoot()
	// Both roots should have all top-level commands, including `das`.
	for _, root := range []*cobra.Command{r1, r2} {
		names := map[string]bool{}
		for _, c := range root.Commands() {
			names[c.Use] = true
		}
		for _, want := range []string{"init", "validate", "doctor", "das", "run"} {
			require.True(t, names[want], "root missing %s", want)
		}
	}
}
