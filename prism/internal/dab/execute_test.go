package dab_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/dab"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// fixtureDASCustomerCurrent creates a typed DAS staging table for the
// adventure_works.customer__current entity matching the fixture contract.
const fixtureDASCustomerCurrent = `
CREATE SCHEMA IF NOT EXISTS das__adventure_works;
CREATE TABLE das__adventure_works.customer__current (
    customer_id     BIGINT    NOT NULL,
    company_name    VARCHAR,
    lifetime_value  DOUBLE,
    modified_date   TIMESTAMP NOT NULL,
    _loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func TestExecute_SingleSourceSingleAttribute(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, modified_date)
		VALUES
		    (1, 'Acme',     1000.0, TIMESTAMP '2024-01-15 10:00:00'),
		    (2, 'Globex',    500.0, TIMESTAMP '2024-02-20 11:00:00');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateFocal(f))

	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{
		EntityID: "customer", Path: "ignored", Focal: f,
	})
	require.NoError(t, err)
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// Two distinct customers → two focal rows.
	rows, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows.Close()
	rows.Next()
	var n int
	require.NoError(t, rows.Scan(&n))
	require.Equal(t, 2, n)

	// IDFR table: two rows, both 'Y'.
	rows2, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__idfr WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows2.Close()
	rows2.Next()
	require.NoError(t, rows2.Scan(&n))
	require.Equal(t, 2, n)

	// Descriptor table: 4 rows (2 customers × 2 outer attributes).
	rows3, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__descriptor WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows3.Close()
	rows3.Next()
	require.NoError(t, rows3.Scan(&n))
	require.Equal(t, 4, n)

	// __current view: typed columns by attribute.
	rows4, err := eng.Query(ctx, `
		SELECT customer_name, customer_lifetime_value__amount, customer_lifetime_value__currency
		FROM dab.customer__current
		ORDER BY customer_name;
	`)
	require.NoError(t, err)
	defer rows4.Close()
	type row struct {
		Name     string
		Amount   float64
		Currency string
	}
	var got []row
	for rows4.Next() {
		var r row
		require.NoError(t, rows4.Scan(&r.Name, &r.Amount, &r.Currency))
		got = append(got, r)
	}
	require.Equal(t, []row{
		{Name: "Acme", Amount: 1000.0, Currency: "USD"},
		{Name: "Globex", Amount: 500.0, Currency: "USD"},
	}, got)
}
