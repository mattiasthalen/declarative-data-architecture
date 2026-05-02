// prism/internal/cli/das_build_test.go
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

// TestDasBuildAgainstFixtureLake builds the DAS layer for the valid fixture
// against an in-memory DuckDB, using a tiny `_lake/` populated from the JSONL fixture.
func TestDasBuildAgainstFixtureLake(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "contracts/das/adventure_works"), 0o755))
	// copy the valid fixture into the temp repo
	for _, p := range []string{"_source.yml", "customer.yml"} {
		src := filepath.FromSlash("../../testdata/contracts/valid/das/adventure_works/" + p)
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			filepath.Join(repoRoot, "contracts/das/adventure_works", p),
			data, 0o644))
	}
	// stage the JSONL under <lake>/<source>/<entity-name>/
	lakeDir := filepath.Join(repoRoot, "_lake/das/adventure_works/Customer")
	require.NoError(t, os.MkdirAll(lakeDir, 0o755))
	jsonl, err := os.ReadFile(filepath.FromSlash("../../testdata/jsonl/customer_v1.jsonl.gz"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lakeDir, "00000.jsonl.gz"), jsonl, 0o644))

	// open in-memory DuckDB and run the build directly (bypass the cobra layer
	// for unit-test speed; we exercise the cobra layer in the E2E test).
	e, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer e.Close()

	var buf bytes.Buffer
	err = RunDasBuild(context.Background(), e, &buf,
		filepath.Join(repoRoot, "contracts"),
		filepath.Join(repoRoot, "_lake"),
		"adventure_works", false,
	)
	require.NoError(t, err, buf.String())

	// Quick sanity: __current returns 2 distinct customer_id values
	rows, qErr := e.Query(context.Background(), `SELECT count(*) FROM "das__adventure_works"."customer__current"`)
	require.NoError(t, qErr)
	defer rows.Close()
	require.True(t, rows.Next())
	var n int
	require.NoError(t, rows.Scan(&n))
	assert.Equal(t, 2, n)
}
