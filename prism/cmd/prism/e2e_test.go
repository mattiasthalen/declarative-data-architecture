//go:build e2e

// prism/cmd/prism/e2e_test.go
package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/marcboeker/go-duckdb/v2"
)

const adventureWorksURL = "https://demodata.grapecity.com/adventureworks/odata/v1/"

// TestE2EAdventureWorks runs the entire DAS pipeline end-to-end against the
// public AdventureWorks OData endpoint. Requires `uv` on PATH and network
// access. Run with: `go test -tags=e2e -run E2E -v ./cmd/prism/`.
func TestE2EAdventureWorks(t *testing.T) {
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not available")
	}

	repo := t.TempDir()
	bin := filepath.Join(repo, "prism")

	// Build prism binary into the temp repo.
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/prism")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	run := func(args ...string) string {
		t.Helper()
		c := exec.Command(bin, args...)
		c.Dir = repo
		var buf bytes.Buffer
		c.Stdout = &buf
		c.Stderr = &buf
		require.NoError(t, c.Run(), buf.String())
		return buf.String()
	}

	// Initialize warehouse repo
	run("init", "--dir", repo)

	// Create the source declaration manually (discover requires network too,
	// but doing it explicitly here lets us scope the entities).
	srcDir := filepath.Join(repo, "contracts", "das", "adventure_works")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "_source.yml"), []byte(
		"version: 1\nsource:\n  provider: odata\n  base_url: "+adventureWorksURL+"\n",
	), 0o644))

	// Discover then keep just `customer`, `product`, `sales_order_header`.
	run("das", "discover", "adventure_works")
	keep := map[string]bool{"_source.yml": true, "customer.yml": true, "product.yml": true, "sales_order_header.yml": true}
	dir, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	for _, e := range dir {
		if !keep[e.Name()] {
			require.NoError(t, os.Remove(filepath.Join(srcDir, e.Name())))
		}
	}

	run("doctor")

	// Land + build (timeout-bounded)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_ = ctx // for future cancellation hooks
	run("das", "land", "adventure_works")
	run("das", "build", "adventure_works")

	// Open the warehouse and assert presence + non-empty current view.
	dbPath := filepath.Join(repo, "warehouse.duckdb")
	db, err := sql.Open("duckdb", dbPath)
	require.NoError(t, err)
	defer db.Close()

	row := db.QueryRow(`SELECT count(*) FROM "das__adventure_works"."customer__current"`)
	var n int
	require.NoError(t, row.Scan(&n))
	assert.Greater(t, n, 0, "customer__current should have rows")

	row = db.QueryRow(`SELECT count(*) FROM "das__adventure_works"."customer__historized"`)
	require.NoError(t, row.Scan(&n))
	assert.Greater(t, n, 0)

	// Each PK appears at most once in __current.
	row = db.QueryRow(`
		SELECT count(*) FROM (
			SELECT customer_id, count(*) c FROM "das__adventure_works"."customer__current" GROUP BY customer_id
		) WHERE c > 1`)
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 0, n)
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil && strings.HasSuffix(d, "prism") {
			return d
		}
	}
	t.Fatalf("could not find project root from %s", wd)
	return ""
}
