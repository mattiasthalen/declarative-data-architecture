package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/cli"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// TestDabBuild_AgainstSimpleFixture: end-to-end CLI invocation against the
// dab_simple fixture. Validates that loadAndValidate + Execute work via
// the public command path and that messages print to stdout.
func TestDabBuild_AgainstSimpleFixture(t *testing.T) {
	tmpDir := t.TempDir()
	contractsRoot := filepath.Join(tmpDir, "contracts")
	require.NoError(t, os.MkdirAll(filepath.Join(contractsRoot, "das"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(contractsRoot, "dab"), 0o755))
	require.NoError(t, copyDir(t,
		"../../testdata/contracts/valid/das/adventure_works",
		filepath.Join(contractsRoot, "das", "adventure_works")))
	require.NoError(t, copyFile(t,
		"../../testdata/contracts/valid/dab_simple/customer.yml",
		filepath.Join(contractsRoot, "dab", "customer.yml")))

	wh := filepath.Join(tmpDir, "warehouse.duckdb")
	eng, err := duckdb.Open(wh)
	require.NoError(t, err)
	defer eng.Close()

	// Seed DAS staging table directly (skip M1 land step).
	_ = eng.Exec(context.Background(), `
		CREATE SCHEMA IF NOT EXISTS das__adventure_works;
		CREATE TABLE das__adventure_works.customer__current (
			customer_id     BIGINT    NOT NULL,
			company_name    VARCHAR,
			lifetime_value  DOUBLE,
			modified_date   TIMESTAMP NOT NULL,
			created_at      TIMESTAMP,
			deactivated_at  TIMESTAMP,
			_loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, modified_date, created_at, deactivated_at)
		VALUES (1, 'Acme', 100.0, TIMESTAMP '2024-01-01', TIMESTAMP '2023-01-01', NULL);
	`)

	var out bytes.Buffer
	require.NoError(t, cli.RunDabBuild(context.Background(), eng, &out, contractsRoot, "customer"))
	require.Contains(t, out.String(), "built dab.customer")
}

func copyDir(t *testing.T, src, dst string) error {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(t *testing.T, src, dst string) error {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
