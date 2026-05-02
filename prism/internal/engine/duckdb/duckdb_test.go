package duckdb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	e, err := Open(":memory:")
	require.NoError(t, err)
	defer e.Close()
	d := e.Dialect()

	cols := []engine.Column{
		{SourcePath: "CustomerID", TargetName: "customer_id", SQLType: "BIGINT", NotNull: true},
		{SourcePath: "CompanyName", TargetName: "company_name", SQLType: "VARCHAR"},
		{SourcePath: "ModifiedDate", TargetName: "modified_date", SQLType: "TIMESTAMP", NotNull: true},
	}

	require.NoError(t, e.Exec(ctx, d.CreateSchemaIfNotExists("das__adventure_works")))
	require.NoError(t, e.Exec(ctx, d.CreateHistorizedTableIfNotExists(engine.HistorizedTableSpec{
		Schema: "das__adventure_works", Name: "customer__historized", Columns: cols,
	})))

	abs, err := filepath.Abs("../../../testdata/jsonl/customer_v1.jsonl.gz")
	require.NoError(t, err)
	require.NoError(t, e.Exec(ctx, d.AppendIntoHistorized(engine.HistorizedAppendSpec{
		Schema: "das__adventure_works", Name: "customer__historized",
		LakeGlob: abs, Compression: "gzip", Columns: cols,
	})))

	// Three rows in fixture, one is a true update of customer_id=1 → all three are unique by hash.
	row := e.db.QueryRowContext(ctx, `SELECT count(*) FROM "das__adventure_works"."customer__historized"`)
	var n int
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 3, n)

	// Idempotency: re-run append, count still 3.
	require.NoError(t, e.Exec(ctx, d.AppendIntoHistorized(engine.HistorizedAppendSpec{
		Schema: "das__adventure_works", Name: "customer__historized",
		LakeGlob: abs, Compression: "gzip", Columns: cols,
	})))
	row = e.db.QueryRowContext(ctx, `SELECT count(*) FROM "das__adventure_works"."customer__historized"`)
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 3, n)

	// Build current view; expect one row per PK.
	require.NoError(t, e.Exec(ctx, d.CreateOrReplaceCurrentView(engine.CurrentViewSpec{
		Schema: "das__adventure_works", Name: "customer__current",
		HistorizedTable: "customer__historized", PrimaryKey: []string{"customer_id"},
	})))
	row = e.db.QueryRowContext(ctx, `SELECT count(*) FROM "das__adventure_works"."customer__current"`)
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 2, n, "two distinct customer_id values → two rows in __current")

	// The latest row for customer_id=1 should be "Acme Updated".
	row = e.db.QueryRowContext(ctx, `SELECT company_name FROM "das__adventure_works"."customer__current" WHERE customer_id = 1`)
	var name string
	require.NoError(t, row.Scan(&name))
	assert.Equal(t, "Acme Updated", name)
}
