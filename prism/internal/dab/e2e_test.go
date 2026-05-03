//go:build e2e

package dab_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

// TestE2E_AdventureWorks_DAB exercises the full DAS+DAB pipeline against the
// AdventureWorks OData feed (same env as M1's e2e). Requires uv and network.
func TestE2E_AdventureWorks_DAB(t *testing.T) {
	if os.Getenv("PRISM_E2E") == "" {
		t.Skip("PRISM_E2E not set; skipping network-bound e2e")
	}
	tmp := t.TempDir()
	prismBin, err := exec.LookPath("prism")
	if err != nil {
		// Fall back to building locally.
		prismBin = filepath.Join(tmp, "prism")
		out, err := exec.Command("go", "build", "-o", prismBin, "../../cmd/prism").CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run := func(name string, args ...string) {
		cmd := exec.Command(prismBin, args...)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s: %s", name, string(out))
		t.Logf("%s:\n%s", name, string(out))
	}
	run("init", "init", "--dir", ".")
	run("das discover", "das", "discover",
		"--source", "adventure_works",
		"--metadata-url", "https://services.odata.org/V4/AdventureWorksV3/AdventureWorks.svc/$metadata")
	run("das run", "das", "run", "--all")
	run("dab discover", "dab", "discover")
	// Inspect: at minimum, there should now be a contracts/dab/customer.yml.
	_, err = os.Stat(filepath.Join(tmp, "contracts", "dab", "customer.yml"))
	require.NoError(t, err)

	run("validate", "validate")
	run("dab run", "dab", "run", "--all")

	// Open the warehouse and assert at least one focal row was produced.
	eng, err := duckdb.Open(filepath.Join(tmp, "warehouse.duckdb"))
	require.NoError(t, err)
	defer eng.Close()
	rows, err := eng.Query(context.Background(),
		`SELECT count(*) FROM dab.customer WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows.Close()
	rows.Next()
	var n int
	require.NoError(t, rows.Scan(&n))
	require.Greater(t, n, 0, "no rows in dab.customer after e2e build")
}
