# Prism M1 — DAS layer design

**Status:** Approved (brainstorm 2026-05-01).
**Roadmap context:** [docs/superpowers/roadmap.md](../roadmap.md).
**Decision records:** [docs/adr/](../../adr/).

## Goal

Ship a working `prism` Go binary that, given a checked-in `contracts/das/<source>.yml`, produces a queryable DuckDB warehouse façade over a JSONL file archive — with no Python tooling visible to the user beyond `uv` itself. AdventureWorks OData is the validation dataset.

## Success criteria

A clean clone of a warehouse repo with one DAS contract for AdventureWorks runs end-to-end:

```bash
prism init
# author contracts/das/adventure_works.yml
prism doctor                       # all green
prism run                          # = prism das run --all (just DAS in M1)
duckdb warehouse.duckdb -c "FROM das__adventure_works.customer__stage LIMIT 5;"
```

…and yields rows of typed, JSON-parsed records pulled from compressed JSONL files in `_lake/das/adventure_works/`.

## Architecture

```
                  ┌──────────────────────────────────┐
                  │     User-edited (git-tracked)    │
                  ├──────────────────────────────────┤
                  │  contracts/das/<source>.yml      │
                  └──────────────┬───────────────────┘
                                 │
                  ┌──────────────▼───────────────────┐
                  │           prism (Go)             │
                  │  ┌─────────────────────────────┐ │
                  │  │  YAML loader + validator    │ │
                  │  │  Engine interface           │ │
                  │  │   └─ DuckDB engine (cgo)    │ │
                  │  │  SQL template library       │ │
                  │  │  uv subprocess orchestrator │ │
                  │  │  embedded prism_dlt_runner  │ │
                  │  └─────────────────────────────┘ │
                  └──┬─────────────────────────┬─────┘
                     │ uv run                  │ go-duckdb
                     ▼                         ▼
        ┌──────────────────────┐    ┌─────────────────────────┐
        │ _pipelines/<source>/ │    │     warehouse.duckdb    │
        │  pyproject.toml      │    │                         │
        │  uv.lock             │    │  schema  das__<source>  │
        │  (venv with dlt[…])  │    │   ├─ <entity>__stage    │
        └────────┬─────────────┘    │   │   (read_json_auto)  │
                 │ writes JSONL     │   …                     │
                 ▼                  └─────────────────────────┘
        ┌──────────────────────┐              ▲
        │  _lake/das/<source>/ │              │ read_json_auto
        │   <entity>/          │──────────────┘
        │     *.jsonl.gz       │
        └──────────────────────┘
```

**Three moving parts:**

1. **`prism` (Go binary)** — single user-facing CLI. Reads YAML contracts, owns the SQL template library, orchestrates dlt subprocesses, talks to DuckDB through the `Engine` interface. The shared `prism_dlt_runner` Python module is `go:embed`ed into the binary and extracted to `~/.cache/prism/dlt_runner/<version>/` on first use.
2. **uv-managed Python venvs** (`_pipelines/<source>/`) — one per source. Contain only `pyproject.toml` + `uv.lock`. Every venv runs the same shared runner. Gitignored.
3. **DuckDB warehouse** (`warehouse.duckdb`) — single file. Holds one schema per source in M1: `das__<source>`, containing only views.

See [ADR-001](../../adr/0001-single-go-binary-and-uv-venvs.md), [ADR-006](../../adr/0006-shared-dlt-runner-embedded-in-binary.md).

## Repository layout

```
declarative-data-architecture/
├── contracts/
│   └── das/
│       └── adventure_works.yml         # one file per source
├── _lake/                              # gitignored
│   └── das/
│       └── adventure_works/
│           ├── Customer/
│           ├── SalesOrderHeader/
│           └── …
├── _pipelines/                         # gitignored
│   └── adventure_works/
│       ├── pyproject.toml
│       └── uv.lock
├── warehouse.duckdb                    # gitignored
├── prism.yml                           # optional repo config
└── docs/
    ├── adr/
    └── superpowers/
        ├── roadmap.md
        └── specs/2026-05-01-prism-m1-das-design.md
```

The Go binary `prism` lives in a separate repo. This repo holds only contracts and outputs.

## YAML shapes

### `prism.yml` (repo-level, optional)

Whole file is optional. Empty repo with sensible defaults works.

```yaml
version: 1
warehouse:
  duckdb_path: ./warehouse.duckdb     # default; override if needed
paths:
  contracts: ./contracts              # default
  lake:      ./_lake                  # default
  pipelines: ./_pipelines             # default
```

### `contracts/das/<source>.yml`

```yaml
version: 1
source:
  provider: odata
  base_url: https://demodata.grapecity.com/adventureworks/odata/v1/
entities:
  - name: Customer
  - name: SalesOrderHeader
    incremental:                      # optional; cursor-based incremental land
      cursor: ModifiedDate
      strategy: append                # or 'replace' for full snapshots
  - name: Product
  # …add one entry per entity to land
```

**Notes:**

- **Source ID is derived from the filename** — `adventure_works.yml` → `adventure_works`. No redundant `id:` field; renaming the file renames the source. Validation enforces snake_case basenames.
- **One file per source, not per entity.** With Shape Y (no per-column DAS schema, see [ADR-003](../../adr/0003-das-owns-no-materialized-data.md)), each entity declaration is essentially just a name. A single list per source is more ergonomic than 70 separate files.
- **`provider:`** drives which dlt source factory the runner instantiates. M1 supports only `odata`. New providers in M2/M3 are added to the runner's dispatch table.
- **dlt invariants** (`max_table_nesting=0`, `add_dlt_id`, `add_dlt_load_id`, `naming_convention=direct`) are **runner-enforced**, not YAML-configurable. See [ADR-002](../../adr/0002-no-dlt-normalization.md).
- **dlt extras** (`dlt[filesystem]`, etc.) are derived from `provider:` via a static mapping in the runner; not exposed as a YAML key.

## CLI surface

| Command | Purpose |
|---|---|
| `prism init` | Scaffold a new warehouse repo: `prism.yml` (default), `contracts/`, `.gitignore`. |
| `prism validate` | Pure YAML/schema check. No DB calls, no network. CI-friendly, pre-commit-friendly. |
| `prism doctor` | Verify `uv` on PATH, DuckDB file writable, contracts parse. |
| `prism das discover <source>` | Fetch upstream schema (e.g., OData `$metadata`); print available entities + types. `--write` saves a reference YAML to `_discovery/<source>/` (gitignored). Informational only — does **not** generate contracts. |
| `prism das land [<source>] [--all]` | Ensure uv venv exists for the source, then run shared dlt runner → JSONL into `_lake/`. |
| `prism das build [<source>] [--all]` | Generate SQL from contracts, apply to DuckDB → `__stage` views. Idempotent. |
| `prism das run [<source>] [--all]` | Convenience: `land` then `build`. |
| `prism run` | Headline command. M1 = `das run --all`. M2 adds DAB; M3 adds DAR. |

## DuckDB output

For each entity in each source, `prism das build` applies exactly two SQL operations. Both idempotent. **DAS materializes nothing.** See [ADR-003](../../adr/0003-das-owns-no-materialized-data.md).

### 1. Ensure schema

```sql
CREATE SCHEMA IF NOT EXISTS das__{source};
```

### 2. Stage view — schema-on-read façade over the JSONL archive

```sql
CREATE OR REPLACE VIEW das__{source}.{entity}__stage AS
SELECT *
FROM read_json_auto(
    '{lake_path}/{source}/{entity}/**/*.jsonl.gz',
    format          = 'newline_delimited',
    compression     = 'gzip',
    union_by_name   = true            -- tolerate additive schema drift
);
```

The view exposes whatever's in the file archive at query time, including `_dlt_id` and `_dlt_load_id`, which DAB can use for incremental processing.

### Where historization lives

| Layer | Form | Owner |
|---|---|---|
| **Archive** (every observation, ever) | Compressed JSONL files in `_lake/` | dlt's filesystem destination |
| **Stage** (queryable façade) | Views over the archive | DAS (`prism das build`) |
| **Bi-temporal projections** (typed, deduped, business-meaningful) | Tables: descriptors with `EFF_TMSTP` / `VER_TMSTP` / `SEQ_NBR`, focal IDFRs | DAB (M2) |

For full-refresh sources, the archive contains every snapshot; DAB will deduplicate when projecting into descriptor tables. For incremental sources, dlt only lands deltas, so the archive is naturally a change log.

## Internals

### Engine interface (Go)

Small surface, only what M1 needs. Grows as M2/M3 land. See [ADR-004](../../adr/0004-go-duckdb-over-adbc.md) for the driver-choice rationale.

```go
// internal/engine/engine.go
package engine

import "context"

type Engine interface {
    Close() error
    Exec(ctx context.Context, sql string) error
    Query(ctx context.Context, sql string) (Rows, error)
    Dialect() Dialect
}

type Dialect interface {
    QuoteIdent(name string) string
    Schema(name string) string
    CreateSchemaIfNotExists(name string) string
    CreateOrReplaceStageView(spec StageViewSpec) string
}

type StageViewSpec struct {
    Schema      string
    Name        string
    LakeGlob    string
    Format      string
    Compression string
}
```

DuckDB implementation: `internal/engine/duckdb/duckdb.go` — opens via `database/sql` + `go-duckdb` (cgo), `Dialect()` returns a struct whose methods produce SQL via Go `text/template`.

The interface deliberately doesn't expose "ingest JSONL" or "build historized" — DAS doesn't need them, and we shouldn't predict M2/M3's needs. Each layer adds methods to `Dialect` as it's built.

### Shared dlt runner (Python)

The runner is a Python module that ships **inside the `prism` Go binary** via `go:embed`. On first use, prism extracts it to `~/.cache/prism/dlt_runner/<version>/`. Versioned by the prism release; never modified by users. See [ADR-006](../../adr/0006-shared-dlt-runner-embedded-in-binary.md).

```
runtime/dlt_runner/                       # in prism Go repo, embedded into the binary
├── __main__.py                           # entry point
├── runner.py                             # contract loading, dispatch, event emission
├── providers/
│   └── odata.py                          # builds dlt source for OData
│   # M2/M3: rest_api.py, sql_database.py, …
├── events.py                             # structured stdout emitter
└── pyproject.toml.tmpl                   # template for _pipelines/<source>/pyproject.toml
```

The runner reads the source contract, dispatches by `provider:` to the appropriate factory, configures dlt with prism's invariants, runs the pipeline, and emits events to stdout.

### Runner invariants

```python
# runtime/dlt_runner/runner.py — invariants, not configurable
PRISM_INVARIANTS = dict(
    write_disposition="append",
    loader_file_format="jsonl",
    max_table_nesting=0,
    naming_convention="direct",
    add_dlt_id=True,
    add_dlt_load_id=True,
)

PROVIDER_EXTRAS = {
    "odata":        ["filesystem"],   # dlt's REST helper handles OData fine
    # M2/M3:
    # "rest_api":    ["filesystem"],
    # "sql_database":["filesystem", "sql_database"],
}
```

### uv venv lifecycle (per source)

When `prism das land <source>` runs:

1. **Synthesize `pyproject.toml`** at `_pipelines/<source>/pyproject.toml` if missing or if the derived extras list has changed since last run. Includes `dlt[filesystem]` plus provider-specific extras plus a path-dep on the embedded runner.
2. **`uv sync --project _pipelines/<source>`** — ensures the venv is current. Fast on no-op.
3. **`uv run --project _pipelines/<source> python -m prism_dlt_runner --source contracts/das/<source>.yml --lake _lake`** — runs the pipeline.
4. Prism captures the runner's stdout, parses events line-by-line, surfaces progress.

`_pipelines/<source>/` is gitignored — pure machine state, regenerable from contracts. `uv.lock` is gitignored by default in M1; reproducibility-via-commit is a post-M1 toggle.

### IPC: structured stdout events

Runner emits one JSON object per line on stdout:

```jsonl
{"event":"source.start","source":"adventure_works","ts":"2026-05-01T12:00:00Z"}
{"event":"entity.start","entity":"Customer"}
{"event":"entity.progress","entity":"Customer","rows":5000}
{"event":"entity.end","entity":"Customer","rows":12345,"load_id":"1735689600.123","files":2}
{"event":"source.end","source":"adventure_works","entities":42,"duration_ms":48201}
{"event":"error","entity":"Product","kind":"http_404","message":"…"}
```

Prism parses, renders progress (per-entity counters), and treats any `error` event (or non-zero exit code) as failure. Non-JSON lines on stdout are warnings; stderr passes through verbatim.

## State, simplified

Because DAS materializes nothing, **prism keeps no state file of its own.** Two stateful surfaces, each owned by a tool that already does it well:

- **dlt** owns incremental cursors (its own state, scoped to each `_pipelines/<source>/` venv).
- **DuckDB** holds whatever DAB and DAR materialize (M2/M3 only).

Re-running `prism das build` after editing a source contract is always safe: missing entities → no view created (orphans persist until pruned); new entities → fresh view; existing entities → view replaced. No reconciliation logic.

## Doctor checks

`prism doctor` verifies, in order:

- `uv` on `$PATH` (and version satisfies floor)
- DuckDB file path is writable
- All contracts under `contracts/das/` parse against the embedded JSON Schema
- For each declared source, `_pipelines/<source>/` is reachable (warns, doesn't fail, if the venv hasn't been synced yet)

## Testing

Three tiers, mapped to where they catch problems.

### Tier 1 — Static validation (no DB, no network, no Python)

`prism validate` runs:

| Check | What |
|---|---|
| YAML parse | Each contract file parses as YAML |
| Schema match | Each parses against the embedded JSON Schema for its layer (`das_v1`) |
| Filename invariants | snake_case, matches `<source>.yml` pattern |
| Entity name uniqueness | No duplicate entity names within a source |
| Provider known | `provider:` is in the runner's dispatch table |
| Required provider fields | e.g., `base_url` for OData, declared per-provider |

CI-friendly. Pre-commit-friendly. Runs in milliseconds.

### Tier 2 — Wiring tests (DB roundtrip, mocked land)

Go-level tests inside the `prism` Go repo:

| Test | What |
|---|---|
| SQL template snapshots | Render every template for representative inputs; assert against committed golden files. Catches accidental dialect drift. |
| Engine round-trip | Open a temp DuckDB, apply rendered DDL, verify schema/view exists with expected name. |
| Runner dispatch (mocked) | Python-side test of the runner's provider dispatch using a fake `dlt.source` — verifies invariants are passed and event JSON is well-formed. |
| CLI smoke tests | `prism init`, `prism validate` on fixtures, `prism doctor` — exit codes, stdout/stderr shape. |

Fast. Runs in CI per commit. Doesn't touch the network.

### Tier 3 — End-to-end against AdventureWorks

A single integration test, gated behind `--e2e` (or env var), runs the full pipeline against the live OData endpoint:

```
prism init  →  drop a contracts/das/adventure_works.yml with 3 entities  →
prism doctor  →  prism das land adventure_works  →  prism das build adventure_works  →
assert _lake/das/adventure_works/{Customer,Product,SalesOrderHeader}/ has *.jsonl.gz
assert das__adventure_works.customer__stage exists and SELECT count(*) > 0
```

Run on schedule (nightly) and on release tags, not per PR. AdventureWorks is a public demo dataset, so this is fine; tests for sources requiring auth will need fixtures or mocked HTTP later.

### Test fixtures

A `testdata/` tree in the prism Go repo:
- `valid/<scenario>.yml` contracts that validate clean.
- `invalid/<scenario>.yml` contracts each violating one rule.
- A handful of small `*.jsonl.gz` files mimicking dlt output (with `_dlt_id`, `_dlt_load_id`) for engine round-trip tests.

## What M1 deliberately does NOT include

- **Per-column schema in DAS contracts** — pushed to DAB. See [ADR-003](../../adr/0003-das-owns-no-materialized-data.md).
- **Materialized historized table at DAS** — JSONL archive is the historization. See [ADR-003](../../adr/0003-das-owns-no-materialized-data.md).
- **User-overridable dlt invariants** — locked in the runner. See [ADR-002](../../adr/0002-no-dlt-normalization.md).
- **Separate prism state file** — dlt + DuckDB suffice.
- **Auto-pruning of orphan stage views** — explicit `prism das prune` deferred to M4.
- **Multi-warehouse / concurrent runs** — DuckDB single-file exclusive lock; single-process assumption.
- **Auth frameworks** — first auth'd source (likely M2) forces the design.

## Known caveats

- **JSON key-ordering stability** — `_record_hash` (introduced in DAB, not DAS) will assume providers return stable key order; if a provider doesn't, DAB's hashing must canonicalize keys. Note for M2.
- **`uv.lock` commit policy** — gitignored by default in M1. Toggle is post-M1.
- **cgo cross-compilation** — `go-duckdb` requires a C toolchain at build time. CI uses `goreleaser` with cross-compilation; release binaries are pre-built per platform. `go install` from source needs the user's local C toolchain — fine for early adopters; document it.
- **Concurrent `prism run`** — DuckDB single-file is exclusive-lock. Two concurrent prism processes against the same warehouse error out. Expected behavior.

## Seams for M2

DAB will plug in by:

1. Adding a layer namespace — `contracts/dab/` alongside `contracts/das/`.
2. Reading from `das__<source>.<entity>__stage` — the only DAS surface DAB consumes. The stage view exposes DuckDB-inferred columns from `read_json_auto` (typed scalars plus STRUCT/LIST for nested fields, plus `_dlt_id` and `_dlt_load_id`). DAB's mapping syntax for projecting these into descriptor attributes is an M2 design question.
3. Extending the `Engine` and `Dialect` interfaces with whatever DAB-specific operations turn out to need engine-level abstraction (likely `MergeDescriptor`, hash-function abstraction).
4. Adding a layer to `prism run` — `das run --all → dab run --all`.

Nothing in M1 forecloses anything M2 needs.
