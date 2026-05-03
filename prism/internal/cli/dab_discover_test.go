package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/cli"
)

func TestDabDiscover_ScaffoldsFromDAS(t *testing.T) {
	tmp := t.TempDir()
	contractsRoot := filepath.Join(tmp, "contracts")
	dasDir := filepath.Join(contractsRoot, "das", "adventure_works")
	require.NoError(t, os.MkdirAll(dasDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "_source.yml"), []byte(`
version: 1
source:
  provider: odata
  base_url: "https://example/odata"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "customer.yml"), []byte(`
version: 1
entity:
  name: Customer
schema:
  primary_key: [customer_id]
  columns:
    - {source_path: CustomerID, target_name: customer_id, type: BIGINT, mode: REQUIRED}
    - {source_path: Name, target_name: name, type: STRING, mode: NULLABLE}
    - {source_path: ModifiedDate, target_name: modified_date, type: TIMESTAMP, mode: REQUIRED}
`), 0o644))

	require.NoError(t, cli.RunDabDiscover(context.Background(), os.Stderr, contractsRoot))

	body, err := os.ReadFile(filepath.Join(contractsRoot, "dab", "customer.yml"))
	require.NoError(t, err)
	got := string(body)
	require.Contains(t, got, "id: CUSTOMER")
	require.Contains(t, got, "id: NAME")
	require.Contains(t, got, "type: STRING")
	require.Contains(t, got, "id: CUSTOMER_ID")
	require.Contains(t, got, "type: NUMBER")
	require.Contains(t, got, "source: adventure_works")
	require.Contains(t, got, "entity: customer")
}

func TestDabDiscover_DoesNotOverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	contractsRoot := filepath.Join(tmp, "contracts")
	dasDir := filepath.Join(contractsRoot, "das", "adventure_works")
	require.NoError(t, os.MkdirAll(dasDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(contractsRoot, "dab"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "_source.yml"), []byte("version: 1\nsource: {provider: odata, base_url: x}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "customer.yml"), []byte(`
version: 1
entity: {name: Customer}
schema:
  primary_key: [customer_id]
  columns:
    - {source_path: CustomerID, target_name: customer_id, type: BIGINT, mode: REQUIRED}
`), 0o644))
	existing := []byte("version: 1\nentity: {id: CUSTOMER, name: CUSTOMER, definition: x}\nattributes: [{id: X, definition: x, type: STRING}]\nmapping_groups: []\n")
	require.NoError(t, os.WriteFile(filepath.Join(contractsRoot, "dab", "customer.yml"), existing, 0o644))

	require.NoError(t, cli.RunDabDiscover(context.Background(), os.Stderr, contractsRoot))

	got, err := os.ReadFile(filepath.Join(contractsRoot, "dab", "customer.yml"))
	require.NoError(t, err)
	require.Equal(t, existing, got)
}
