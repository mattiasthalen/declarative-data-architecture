// prism/internal/drift/drift_test.go
package drift

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/engine"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

func TestDetectNoDrift(t *testing.T) {
	e, abs := setupCustomerHistorized(t, false /* no drift */)
	defer e.Close()
	res, err := DetectNullsInRequired(context.Background(), e, "das__adventure_works", "customer__historized", reqColumns())
	require.NoError(t, err)
	assert.Empty(t, res, abs)
}

func TestDetectDriftRequiredNull(t *testing.T) {
	e, _ := setupCustomerHistorized(t, true /* inject NULL */)
	defer e.Close()
	res, err := DetectNullsInRequired(context.Background(), e, "das__adventure_works", "customer__historized", reqColumns())
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "modified_date", res[0].Column)
	assert.Greater(t, res[0].NullCount, 0)
}

func reqColumns() []engine.Column {
	cs, _ := loadContractCols(filepath.FromSlash("../../testdata/contracts/valid/das/adventure_works/customer.yml"))
	return cs
}

func loadContractCols(path string) ([]engine.Column, error) {
	ent, err := contracts.LoadEntity(path)
	if err != nil {
		return nil, err
	}
	var out []engine.Column
	for _, c := range ent.Schema.Columns {
		out = append(out, engine.Column{
			TargetName: c.TargetName, NotNull: c.Mode == "REQUIRED",
		})
	}
	return out, nil
}

// setupCustomerHistorized creates a tiny historized table; injectDrift adds a
// row whose REQUIRED column modified_date is NULL (simulating a source path
// gone missing).
func setupCustomerHistorized(t *testing.T, injectDrift bool) (*duckdb.Engine, string) {
	t.Helper()
	e, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, e.Exec(ctx, `CREATE SCHEMA "das__adventure_works"`))
	require.NoError(t, e.Exec(ctx, `CREATE TABLE "das__adventure_works"."customer__historized" (
		_record_hash VARCHAR PRIMARY KEY,
		_dlt_id VARCHAR, _dlt_load_id VARCHAR, _loaded_at TIMESTAMP,
		customer_id BIGINT NOT NULL,
		company_name VARCHAR,
		modified_date TIMESTAMP
	)`))
	require.NoError(t, e.Exec(ctx, `INSERT INTO "das__adventure_works"."customer__historized"
		VALUES ('h1','d1','L1', CURRENT_TIMESTAMP, 1, 'Acme', '2026-01-01')`))
	if injectDrift {
		require.NoError(t, e.Exec(ctx, `INSERT INTO "das__adventure_works"."customer__historized"
			VALUES ('h2','d2','L2', CURRENT_TIMESTAMP, 2, 'Beta', NULL)`))
	}
	return e, "ok"
}
