package duckdb

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine"
)

var update = flag.Bool("update", false, "regenerate golden files")

func goldenAssert(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".sql")
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got+"\n"), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file %s; run `go test ./internal/tmpl/duckdb/ -update`", path)
	assert.Equal(t, string(want), got+"\n")
}

func TestCreateSchema(t *testing.T) {
	got, err := RenderCreateSchema("das__adventure_works")
	require.NoError(t, err)
	goldenAssert(t, "create_schema", got)
}

func sampleColumns() []engine.Column {
	return []engine.Column{
		{SourcePath: "CustomerID", TargetName: "customer_id", SQLType: "BIGINT", NotNull: true},
		{SourcePath: "CompanyName", TargetName: "company_name", SQLType: "VARCHAR", NotNull: false},
		{SourcePath: "ModifiedDate", TargetName: "modified_date", SQLType: "TIMESTAMP", NotNull: true},
	}
}

func TestCreateHistorized(t *testing.T) {
	got, err := RenderCreateHistorized(engine.HistorizedTableSpec{
		Schema:  "das__adventure_works",
		Name:    "customer__historized",
		Columns: sampleColumns(),
	})
	require.NoError(t, err)
	goldenAssert(t, "create_historized", got)
}

func TestAppendHistorized(t *testing.T) {
	got, err := RenderAppendHistorized(engine.HistorizedAppendSpec{
		Schema:      "das__adventure_works",
		Name:        "customer__historized",
		LakeGlob:    "/lake/das/adventure_works/Customer/**/*.jsonl.gz",
		Compression: "gzip",
		Columns:     sampleColumns(),
	})
	require.NoError(t, err)
	goldenAssert(t, "append_historized", got)
}

func TestCreateCurrentView(t *testing.T) {
	got, err := RenderCreateCurrentView(engine.CurrentViewSpec{
		Schema:          "das__adventure_works",
		Name:            "customer__current",
		HistorizedTable: "customer__historized",
		PrimaryKey:      []string{"customer_id"},
	})
	require.NoError(t, err)
	goldenAssert(t, "create_current_view", got)
}

// readGolden reads a golden file from testdata and returns its content trimmed.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return strings.TrimSpace(string(b))
}

func TestRenderCreateIdfr_Golden(t *testing.T) {
	got, err := RenderCreateIdfr(engine.IdfrTableSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_idfr.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateFocal_Golden(t *testing.T) {
	got, err := RenderCreateFocal(engine.FocalTableSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_focal.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateDescriptor_Golden(t *testing.T) {
	got, err := RenderCreateDescriptor(engine.DescriptorTableSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_descriptor.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateRelationship_Golden(t *testing.T) {
	got, err := RenderCreateRelationship(engine.RelationshipTableSpec{
		Schema: "dab", Entity: "customer", Related: "order",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_relationship.sql.golden")
	require.Equal(t, want, got)
}
