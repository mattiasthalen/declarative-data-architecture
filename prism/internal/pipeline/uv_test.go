// prism/internal/pipeline/uv_test.go
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUVVersion(t *testing.T) {
	cases := []struct {
		out  string
		want string
	}{
		{"uv 0.5.10\n", "0.5.10"},
		{"uv 0.6.0 (Homebrew 2026-01-01)\n", "0.6.0"},
	}
	for _, c := range cases {
		got, err := parseUVVersion(c.out)
		require.NoError(t, err, c.out)
		assert.Equal(t, c.want, got)
	}
}

func TestVersionAtLeast(t *testing.T) {
	assert.True(t, versionAtLeast("0.5.0", "0.5.0"))
	assert.True(t, versionAtLeast("0.5.10", "0.5.0"))
	assert.True(t, versionAtLeast("1.0.0", "0.5.0"))
	assert.False(t, versionAtLeast("0.4.99", "0.5.0"))
}
