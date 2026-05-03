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
    created_at      TIMESTAMP,
    deactivated_at  TIMESTAMP,
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
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES
		    (1, 'Acme',     1000.0, TIMESTAMP '2023-06-01', NULL,                    TIMESTAMP '2024-01-15 10:00:00'),
		    (2, 'Globex',    500.0, TIMESTAMP '2023-09-01', TIMESTAMP '2024-04-01',  TIMESTAMP '2024-02-20 11:00:00');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab_simple/customer.yml")
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

	// Descriptor table: 6 rows (2 customers × 3 outer attributes).
	rows3, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__descriptor WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows3.Close()
	rows3.Next()
	require.NoError(t, rows3.Scan(&n))
	require.Equal(t, 6, n)

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

func TestExecute_AtomicGroup_WindowMembers(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES
		    (1, 'Acme', 1000.0, TIMESTAMP '2023-06-01', NULL,                       TIMESTAMP '2024-01-15 10:00:00'),
		    (2, 'Globex', 500.0, TIMESTAMP '2023-09-01', TIMESTAMP '2024-04-01',    TIMESTAMP '2024-02-20 11:00:00');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab_simple/customer.yml")
	require.NoError(t, err)
	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: f})
	require.NoError(t, err)
	require.NoError(t, dab.Execute(ctx, eng, plan))

	rows, err := eng.Query(ctx, `
		SELECT customer_name,
		       customer_active_window__start,
		       customer_active_window__end
		FROM dab.customer__current
		ORDER BY customer_name;
	`)
	require.NoError(t, err)
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		var sta, end interface{}
		require.NoError(t, rows.Scan(&name, &sta, &end))
		got = append(got, name)
	}
	require.Equal(t, []string{"Acme", "Globex"}, got)

	// Spot-check the typed view exposes the window columns.
	rows2, err := eng.Query(ctx, `
		SELECT count(*) FROM dab.customer__customer_active_window WHERE row_st = 'Y';
	`)
	require.NoError(t, err)
	defer rows2.Close()
	rows2.Next()
	var n int
	require.NoError(t, rows2.Scan(&n))
	require.Equal(t, 2, n)
}

const fixtureDASOrderCurrent = `
CREATE TABLE das__adventure_works.order__current (
    order_id        BIGINT    NOT NULL,
    customer_id     BIGINT    NOT NULL,
    modified_date   TIMESTAMP NOT NULL,
    _loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const fixtureDASStripeCustomerCurrent = `
CREATE SCHEMA IF NOT EXISTS das__stripe;
CREATE TABLE das__stripe.customer__current (
    stripe_id        VARCHAR  NOT NULL,
    name             VARCHAR,
    total_revenue    BIGINT,
    currency         VARCHAR,
    aw_customer_id   BIGINT   NOT NULL,
    updated          TIMESTAMP NOT NULL,
    _loaded_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func TestExecute_MultiSourceUnification(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, fixtureDASOrderCurrent))
	require.NoError(t, eng.Exec(ctx, fixtureDASStripeCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES (1, 'Acme', 1000.0, TIMESTAMP '2023-06-01', NULL, TIMESTAMP '2024-01-15');
	`))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__stripe.customer__current
		(stripe_id, name, total_revenue, currency, aw_customer_id, updated)
		VALUES ('cus_xyz', 'Acme Corporation', 250000, 'USD', 1, TIMESTAMP '2024-03-01');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: f})
	require.NoError(t, err)
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// One focal row — same canonical IDFR string from both sources collapses to one surrogate.
	row, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer;`)
	require.NoError(t, err)
	defer row.Close()
	row.Next()
	var n int
	require.NoError(t, row.Scan(&n))
	require.Equal(t, 1, n)

	// IDFR table: two rows — both sources emit the same idfr string ('CUSTOMER:1')
	// so they share the same surrogate key (MD5), but their eff_tmstp values differ
	// (AW: 2024-01-15, Stripe: 2024-03-01), so the dedup check (idfr, eff_tmstp)
	// allows both to coexist. One row per source contribution, same surrogate.
	row2, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__idfr;`)
	require.NoError(t, err)
	defer row2.Close()
	row2.Next()
	require.NoError(t, row2.Scan(&n))
	require.Equal(t, 2, n)

	// Both descriptor sources contributed CUSTOMER_NAME and CUSTOMER_LIFETIME_VALUE.
	// ROW_ST = 'Y' selects the latest per (customer_key, type_key) by eff_tmstp.
	// Stripe's updated 2024-03-01 > AW's modified_date 2024-01-15, so stripe wins.
	row3, err := eng.Query(ctx, `
		SELECT customer_name, customer_lifetime_value__amount
		FROM dab.customer__current;
	`)
	require.NoError(t, err)
	defer row3.Close()
	row3.Next()
	var name string
	var amount float64
	require.NoError(t, row3.Scan(&name, &amount))
	require.Equal(t, "Acme Corporation", name)
	require.Equal(t, 2500.0, amount) // 250000 / 100.0 from stripe
}

func TestExecute_Relationships(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, fixtureDASOrderCurrent))
	require.NoError(t, eng.Exec(ctx, fixtureDASStripeCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES (1, 'Acme', 1000.0, TIMESTAMP '2023-06-01', NULL, TIMESTAMP '2024-01-15');
	`))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.order__current
		(order_id, customer_id, modified_date)
		VALUES
		    (101, 1, TIMESTAMP '2024-01-20'),
		    (102, 1, TIMESTAMP '2024-02-05');
	`))

	custFocal, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	orderFocal, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/order.yml")
	require.NoError(t, err)

	custPlan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: custFocal})
	require.NoError(t, err)
	orderPlan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "order", Focal: orderFocal})
	require.NoError(t, err)

	require.NoError(t, dab.Execute(ctx, eng, custPlan))
	require.NoError(t, dab.Execute(ctx, eng, orderPlan))

	// Two rows in the relationship table, ROW_ST = 'Y'.
	row, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__order__rel WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer row.Close()
	row.Next()
	var n int
	require.NoError(t, row.Scan(&n))
	require.Equal(t, 2, n)

	// The customer_key on the relationship table matches the focal table.
	row2, err := eng.Query(ctx, `
		SELECT count(*)
		FROM dab.customer__order__rel r
		JOIN dab.customer c ON c.customer_key = r.customer_key
		JOIN dab.order    o ON o.order_key    = r.order_key
		WHERE r.row_st = 'Y';
	`)
	require.NoError(t, err)
	defer row2.Close()
	row2.Next()
	require.NoError(t, row2.Scan(&n))
	require.Equal(t, 2, n)
}
