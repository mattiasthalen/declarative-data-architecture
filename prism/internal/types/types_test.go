package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTypeBasic(t *testing.T) {
	cases := []struct {
		in      string
		wantSQL string
	}{
		{"STRING", "VARCHAR"},
		{"INTEGER", "INTEGER"},
		{"BIGINT", "BIGINT"},
		{"BOOLEAN", "BOOLEAN"},
		{"DATE", "DATE"},
		{"TIMESTAMP", "TIMESTAMP"},
		{"JSON", "JSON"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			tp, err := Parse(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.wantSQL, tp.DuckDBType())
		})
	}
}

func TestParseDecimal(t *testing.T) {
	tp, err := Parse("DECIMAL(18,4)")
	require.NoError(t, err)
	assert.Equal(t, "DECIMAL(18,4)", tp.DuckDBType())
}

func TestParseDecimalRequiresPrecision(t *testing.T) {
	_, err := Parse("DECIMAL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "precision")
}

func TestParseUnknown(t *testing.T) {
	_, err := Parse("FLOAT64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}
