// prism/internal/discover/odata_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetadata(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("../../testdata/odata/$metadata.xml"))
	require.NoError(t, err)
	entities, err := ParseODataMetadata(data)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	c := entities[0]
	assert.Equal(t, "Customer", c.Name)
	assert.Equal(t, []string{"CustomerID"}, c.Keys)
	require.Len(t, c.Properties, 3)
	assert.Equal(t, "Edm.Int64", c.Properties[0].EDMType)
	assert.False(t, c.Properties[0].Nullable)
}

func TestRenderScaffold(t *testing.T) {
	ent := EntityType{
		Name: "Customer",
		Keys: []string{"CustomerID"},
		Properties: []Property{
			{Name: "CustomerID", EDMType: "Edm.Int64", Nullable: false},
			{Name: "CompanyName", EDMType: "Edm.String", Nullable: true},
		},
	}
	yaml, err := RenderEntityScaffold(ent)
	require.NoError(t, err)
	assert.Contains(t, yaml, "name: Customer")
	assert.Contains(t, yaml, "primary_key:\n    - customer_id")
	assert.Contains(t, yaml, "type: BIGINT")
	assert.Contains(t, yaml, "type: STRING")
	assert.Contains(t, yaml, "mode: REQUIRED")
}
