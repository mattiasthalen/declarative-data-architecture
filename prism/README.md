# prism

Declarative data architecture CLI. Drives a DuckDB warehouse from YAML contracts.

For the architecture spec and ADRs, see your warehouse repo's `docs/superpowers/specs/2026-05-01-prism-m1-das-design.md` and `docs/adr/`.

## Status

M1 (DAS layer) — usable end-to-end against AdventureWorks OData. M2 (DAB) and M3 (DAR) on the roadmap.

## Install

### From release binary (recommended)

Download the appropriate binary from the [Releases](https://github.com/prism-data/prism/releases) page.

### From source

Requires Go 1.22+ and a C toolchain (cgo for go-duckdb).

```bash
go install github.com/prism-data/prism/cmd/prism@latest
```

## External dependencies

`prism` shells out to **`uv`** (Astral's Python package manager) to manage per-source dlt venvs. Install it once:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
```

## Quickstart

```bash
mkdir my-warehouse && cd my-warehouse
prism init
cat > contracts/das/adventure_works/_source.yml <<'YAML'
version: 1
source:
  provider: odata
  base_url: https://demodata.grapecity.com/adventureworks/odata/v1/
YAML
mkdir -p contracts/das/adventure_works
prism das discover adventure_works                 # generates per-entity scaffolds
prism doctor                                       # verify environment
prism run                                          # land + build all sources
duckdb warehouse.duckdb -c "FROM das__adventure_works.customer__current LIMIT 5;"
```

## Commands

| Command | Purpose |
|---|---|
| `prism init` | Scaffold a warehouse repo |
| `prism validate` | Lint contracts (no DB, no network) |
| `prism doctor` | Verify uv, DuckDB, contracts |
| `prism das discover <source>` | Generate per-entity contract scaffolds from upstream metadata |
| `prism das land [<source>]` | Run dlt → JSONL into `_lake/` |
| `prism das build [<source>]` | SQL: typed historized + current |
| `prism das run [<source>]` | land + build |
| `prism run` | M1 alias for `prism das run --all` |

## License

MIT.
