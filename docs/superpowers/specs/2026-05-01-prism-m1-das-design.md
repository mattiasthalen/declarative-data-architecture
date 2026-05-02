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
duckdb warehouse.duckdb -c "FROM das__adventure_works.customer__current LIMIT 5;"
```

…and yields rows of typed, JSON-parsed records pulled from compressed JSONL files in `_lake/das/adventure_works/`.

## Architecture

DAS has **two sub-stages** (per [ADR-003](../../adr/0003-das-owns-no-materialized-data.md)):

- **Landing** — forgiving. dlt writes raw JSONL to `_lake/`.
- **Staging** — strict, contract-driven. Typed DuckDB tables; drift surfaces here.

```
                  ┌──────────────────────────────────────┐
                  │     User-edited (git-tracked)        │
                  ├──────────────────────────────────────┤
                  │  contracts/das/<source>/             │
                  │     _source.yml                      │
                  │     <entity>.yml  (per entity,       │
                  │                    with columns)     │
                  └──────────────┬───────────────────────┘
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
        ┌──────────────────────┐    ┌──────────────────────────────────┐
        │ _pipelines/<source>/ │    │       warehouse.duckdb           │
        │  pyproject.toml      │    │                                  │
        │  uv.lock             │    │  schema  das__<source>           │
        │  (venv with dlt[…])  │    │   ├─ <entity>__historized        │
        └────────┬─────────────┘    │   │   (typed table, append-on-   │
                 │ writes JSONL     │   │    hash)                     │
                 ▼                  │   ├─ <entity>__current           │
        ┌──────────────────────┐    │   │   (view, latest per PK)      │
        │  _lake/das/<source>/ │    │   …                              │
        │   <entity>/          │    └──────────────────────────────────┘
        │     *.jsonl.gz       │              ▲
        │   (landing — raw,    │              │ read_ndjson_objects +
        │    forgiving)        │──────────────┘ typed casts (build)
        └──────────────────────┘
```

**Three moving parts:**

1. **`prism` (Go binary)** — single user-facing CLI. Reads YAML contracts, owns the SQL template library, orchestrates dlt subprocesses, talks to DuckDB through the `Engine` interface. The shared `prism_dlt_runner` Python module is `go:embed`ed into the binary and extracted to `~/.cache/prism/dlt_runner/<version>/` on first use.
2. **uv-managed Python venvs** (`_pipelines/<source>/`) — one per source. Contain only `pyproject.toml` + `uv.lock`. Every venv runs the same shared runner. Gitignored.
3. **DuckDB warehouse** (`warehouse.duckdb`) — single file. Holds one schema per source in M1: `das__<source>`, containing one typed table + one view per declared entity.

See [ADR-001](../../adr/0001-single-go-binary-and-uv-venvs.md), [ADR-003](../../adr/0003-das-owns-no-materialized-data.md), [ADR-006](../../adr/0006-shared-dlt-runner-embedded-in-binary.md).

## Repository layout

```
declarative-data-architecture/
├── contracts/
│   └── das/
│       └── adventure_works/
│           ├── _source.yml             # provider, base_url
│           ├── customer.yml            # per-entity, with columns
│           ├── sales_order_header.yml
│           └── …
├── _lake/                              # gitignored — landed JSONL (raw)
│   └── das/
│       └── adventure_works/
│           ├── Customer/
│           ├── SalesOrderHeader/
│           └── …
├── _pipelines/                         # gitignored — uv venvs
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

### `contracts/das/<source>/_source.yml`

```yaml
version: 1
source:
  provider: odata
  base_url: https://demodata.grapecity.com/adventureworks/odata/v1/
```

### `contracts/das/<source>/<entity>.yml`

```yaml
# contracts/das/adventure_works/customer.yml
version: 1
entity:
  name: Customer                      # name as upstream exposes it (used by dlt)
  description: "Customer master data"  # optional
incremental:                          # optional; cursor-based incremental land
  cursor: ModifiedDate
  strategy: append                    # or 'replace' for full snapshots
schema:
  primary_key:                        # required; one or more column refs (target_name)
    - customer_id
  columns:
    - source_path: CustomerID
      target_name: customer_id
      type: BIGINT
      mode: REQUIRED
      description: "Customer master ID"
    - source_path: CompanyName
      target_name: company_name
      type: VARCHAR
      mode: NULLABLE
    - source_path: ModifiedDate
      target_name: modified_date
      type: TIMESTAMP
      mode: REQUIRED
    # …
```

**Notes:**

- **Source ID is derived from the directory name** — `contracts/das/adventure_works/` → `adventure_works`. **Entity ID is derived from the entity-file basename** — `customer.yml` → `customer` (DuckDB-side). The `entity.name` field is the upstream-system name (e.g., OData entity set), used by dlt for extraction. See [ADR-007](../../adr/0007-one-yaml-per-source.md).
- **One file per entity** (plus `_source.yml`). With per-column schema, single-file-per-source becomes unwieldy at scale.
- **`provider:`** drives which dlt source factory the runner instantiates. M1 supports only `odata`. New providers in M2/M3 are added to the runner's dispatch table.
- **Type system (M1)** is a small canonical set, mapped to DuckDB types in the generator: `STRING` → `VARCHAR`, `INTEGER` → `INTEGER`, `BIGINT` → `BIGINT`, `DECIMAL(p,s)` → `DECIMAL(p,s)`, `BOOLEAN` → `BOOLEAN`, `DATE` → `DATE`, `TIMESTAMP` → `TIMESTAMP`, `JSON` → `JSON`. Extending the set requires a generator change.
- **`mode: REQUIRED`** vs **`NULLABLE`** controls drift detection. A REQUIRED column that comes back NULL after the JSON cast is a drift signal (cast failed or source path missing) — surfaces as a tier-1 contract test failure.
- **dlt invariants** (`max_table_nesting=0`, `add_dlt_id`, `add_dlt_load_id`, `naming_convention=direct`) are **runner-enforced**, not YAML-configurable. See [ADR-002](../../adr/0002-no-dlt-normalization.md).
- **dlt extras** (`dlt[filesystem]`, etc.) are derived from `provider:` via a static mapping in the runner; not exposed as a YAML key.

## CLI surface

| Command | Purpose |
|---|---|
| `prism init` | Scaffold a new warehouse repo: `prism.yml` (default), `contracts/`, `.gitignore`. |
| `prism validate` | Pure YAML/schema check. No DB calls, no network. CI-friendly, pre-commit-friendly. |
| `prism doctor` | Verify `uv` on PATH, DuckDB file writable, contracts parse. |
| `prism das discover <source>` | Fetch upstream schema (e.g., OData `$metadata`); generate per-entity contract scaffolds at `contracts/das/<source>/<entity>.yml`. Skips files that already exist; `--update` does drift detection (added/removed/changed fields) and rewrites with caution. |
| `prism das land [<source>] [--all]` | Ensure uv venv exists for the source, then run shared dlt runner → JSONL into `_lake/`. |
| `prism das build [<source>] [--all]` | Generate SQL from contracts, apply to DuckDB → `__historized` typed tables + `__current` views. Idempotent. |
| `prism das run [<source>] [--all]` | Convenience: `land` then `build`. |
| `prism run` | Headline command. M1 = `das run --all`. M2 adds DAB; M3 adds DAR. |

## DuckDB output

For each entity in each source, `prism das build` applies three SQL operations. All idempotent. See [ADR-003](../../adr/0003-das-owns-no-materialized-data.md).

### 1. Ensure schema

```sql
CREATE SCHEMA IF NOT EXISTS das__{source};
```

### 2. Historized table — typed audit log, append-on-hash

**DDL (idempotent via `IF NOT EXISTS`):**

```sql
CREATE TABLE IF NOT EXISTS das__{source}.{entity}__historized (
    _record_hash  VARCHAR  NOT NULL PRIMARY KEY,
    _dlt_id       VARCHAR  NOT NULL,
    _dlt_load_id  VARCHAR  NOT NULL,
    _loaded_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- contract-declared columns, in declaration order:
    {target_name_1}  {duckdb_type_1}  {NULL_or_NOT_NULL_1},
    {target_name_2}  {duckdb_type_2}  {NULL_or_NOT_NULL_2},
    ...
);
```

`mode: REQUIRED` → `NOT NULL`; `mode: NULLABLE` → omitted (DuckDB default).

**Append (idempotent, dedup by hash):**

```sql
INSERT INTO das__{source}.{entity}__historized (
    _record_hash, _dlt_id, _dlt_load_id, _loaded_at,
    {target_name_1}, {target_name_2}, ...
)
SELECT
    md5(typed_row::VARCHAR)                 AS _record_hash,
    json->>'$._dlt_id'                      AS _dlt_id,
    json->>'$._dlt_load_id'                 AS _dlt_load_id,
    CURRENT_TIMESTAMP                       AS _loaded_at,
    typed_row.{target_name_1},
    typed_row.{target_name_2},
    ...
FROM (
    SELECT
        json,
        struct_pack(
            {target_name_1} := CAST(json_extract(json, '$.{source_path_1}') AS {duckdb_type_1}),
            {target_name_2} := CAST(json_extract(json, '$.{source_path_2}') AS {duckdb_type_2}),
            ...
        ) AS typed_row
    FROM read_ndjson_objects(
        '{lake_path}/{source}/{entity}/**/*.jsonl.gz',
        compression = 'gzip'
    ) AS t(json)
)
ON CONFLICT (_record_hash) DO NOTHING;
```

**What gets hashed:** `md5(typed_row::VARCHAR)` — the typed row struct as text. Two re-lands of the same record produce the same hash (after typing); a real change to any column produces a new hash and a new appended row. `_dlt_id` and `_dlt_load_id` are explicitly excluded from the hash via the `struct_pack` projection (they live in their own columns).

### 3. Current view — latest row per primary key

```sql
CREATE OR REPLACE VIEW das__{source}.{entity}__current AS
SELECT * EXCLUDE (_record_hash, _dlt_id, _dlt_load_id, _loaded_at, _row_num)
FROM (
    SELECT *,
        ROW_NUMBER() OVER (
            PARTITION BY {pk_column_1}, {pk_column_2}, ...
            ORDER BY _loaded_at DESC, _record_hash DESC
        ) AS _row_num
    FROM das__{source}.{entity}__historized
)
WHERE _row_num = 1;
```

`primary_key` from the contract drives the `PARTITION BY`. Tie-breaker `_record_hash DESC` makes ordering deterministic when two rows share `_loaded_at`.

### Where historization lives

| Layer | Form | Owner |
|---|---|---|
| **Landing archive** (every byte ever observed) | Compressed JSONL files in `_lake/` | dlt's filesystem destination |
| **Typed audit log** (every typed observation, deduped on hash) | `das__<source>.<entity>__historized` table | DAS staging (`prism das build`) |
| **Latest snapshot** (typed, one row per PK) | `das__<source>.<entity>__current` view | DAS staging |
| **Bi-temporal projections** (descriptors, focal IDFRs) | Tables with `EFF_TMSTP` / `VER_TMSTP` / `SEQ_NBR` | DAB (M2) |

The JSONL archive is the *raw, pre-typing* audit log. The historized table is the *typed, contract-validated* audit log. They are not redundant — the typed audit log is what makes drift detection meaningful and DAB rebuilds cheap.

### Drift detection

A `prism das build` run that fails (or produces unexpected NULLs in REQUIRED columns) signals upstream change. Tier-1 contract tests assert: for every REQUIRED column, the count of NULL values in `__historized` rows from the latest `_dlt_load_id` is zero. Failures point at the specific contract column whose `source_path` no longer resolves. Fix: update the contract; re-run `prism das build`.

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
    CreateHistorizedTableIfNotExists(spec HistorizedTableSpec) string
    AppendIntoHistorized(spec HistorizedAppendSpec) string
    CreateOrReplaceCurrentView(spec CurrentViewSpec) string
}

type Column struct {
    SourcePath string  // e.g. "CustomerID" or "payload.userId"
    TargetName string  // e.g. "customer_id"
    SQLType    string  // dialect-specific (e.g. "BIGINT", "DECIMAL(18,4)")
    NotNull    bool    // mode: REQUIRED → true
}

type HistorizedTableSpec struct {
    Schema  string
    Name    string         // e.g. "customer__historized"
    Columns []Column
}

type HistorizedAppendSpec struct {
    Schema   string
    Name     string         // e.g. "customer__historized"
    LakeGlob string         // path to JSONL files
    Compression string      // "gzip"
    Columns  []Column
}

type CurrentViewSpec struct {
    Schema           string
    Name             string         // e.g. "customer__current"
    HistorizedTable  string         // e.g. "customer__historized"
    PrimaryKey       []string       // target_name list
}
```

DuckDB implementation: `internal/engine/duckdb/duckdb.go` — opens via `database/sql` + `go-duckdb` (cgo), `Dialect()` returns a struct whose methods produce SQL via Go `text/template`.

The interface scope is intentionally limited to the operations DAS performs in M1 — schema, typed historized DDL, append, current view. M2/M3 will extend the interface (likely `MergeDescriptor`, `UpsertFocal`, hash-function abstraction) when their templates are designed.

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

## State

**Prism keeps no state file of its own.** Two stateful surfaces, each owned by a tool that already does it well:

- **dlt** owns incremental cursors (its own state, scoped to each `_pipelines/<source>/` venv).
- **DuckDB** holds the materialized historized tables and the views derived from them.

Re-running `prism das build` after adding/editing a contract is safe:
- New entity → DDL creates the historized table and current view; first build ingests JSONL.
- Existing entity, no schema change → `IF NOT EXISTS` and `CREATE OR REPLACE VIEW` are no-ops; the append re-scans JSONL and skips already-hashed rows via `ON CONFLICT DO NOTHING`.
- Existing entity, **column added or retyped** → DDL is incompatible with the existing table. M1 fails the build with a clear error message; the user drops the historized table manually (or accepts a future migration command in M4). See "What M1 deliberately does NOT include".
- Removed entity → its tables and views linger until `prism das prune` (M4).

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
| Schema match | Each parses against the embedded JSON Schema for its layer (`das_source_v1`, `das_entity_v1`) |
| Filename invariants | snake_case for source directories and entity files; reserved `_source.yml` per source |
| Entity uniqueness | No duplicate entity files within a source |
| `target_name` uniqueness | No duplicate `target_name` values within an entity contract |
| `primary_key` references | Every `primary_key` entry refers to a declared `target_name` |
| Type names known | Every column `type:` is in the supported set |
| Provider known | `provider:` is in the runner's dispatch table |
| Required provider fields | e.g., `base_url` for OData, declared per-provider |

CI-friendly. Pre-commit-friendly. Runs in milliseconds.

### Tier 2 — Wiring tests (DB roundtrip, mocked land)

Go-level tests inside the `prism` Go repo:

| Test | What |
|---|---|
| SQL template snapshots | Render every template (CreateSchema, CreateHistorizedTable, AppendIntoHistorized, CreateOrReplaceCurrentView) for representative inputs; assert against committed golden files. Catches accidental dialect drift. |
| Engine round-trip | Open a temp DuckDB, apply rendered DDL, verify schema/table/view exist with expected columns and types. Use small `*.jsonl.gz` fixtures to exercise the typed-cast append path. |
| Drift detection | With a fixture JSONL where a `REQUIRED` column's source path is missing, run a build, verify the post-build NULL-in-REQUIRED contract test fails with a useful error message. |
| Hash determinism | Two re-lands of the same source row produce the same `_record_hash`; a single-column change produces a different hash; the `ON CONFLICT DO NOTHING` path correctly skips re-lands. |
| Current-view selection | After multiple loads of overlapping rows, `__current` returns exactly one row per PK, the latest by `_loaded_at`. |
| Runner dispatch (mocked) | Python-side test of the runner's provider dispatch using a fake `dlt.source` — verifies invariants are passed and event JSON is well-formed. |
| CLI smoke tests | `prism init`, `prism validate` on fixtures, `prism doctor` — exit codes, stdout/stderr shape. |

Fast. Runs in CI per commit. Doesn't touch the network.

### Tier 3 — End-to-end against AdventureWorks

A single integration test, gated behind `--e2e` (or env var), runs the full pipeline against the live OData endpoint:

```
prism init  →
prism das discover adventure_works (generates draft entity contracts)  →
trim contracts/das/adventure_works/ down to 3 entities (customer, product, sales_order_header)  →
prism doctor  →  prism das land adventure_works  →  prism das build adventure_works  →
assert _lake/das/adventure_works/{Customer,Product,SalesOrderHeader}/ has *.jsonl.gz
assert das__adventure_works.customer__historized has rows; columns match contract; no NULLs in REQUIRED columns
assert das__adventure_works.customer__current has exactly one row per primary_key
```

Run on schedule (nightly) and on release tags, not per PR. AdventureWorks is a public demo dataset, so this is fine; tests for sources requiring auth will need fixtures or mocked HTTP later.

### Test fixtures

A `testdata/` tree in the prism Go repo:
- `valid/<scenario>.yml` contracts that validate clean.
- `invalid/<scenario>.yml` contracts each violating one rule.
- A handful of small `*.jsonl.gz` files mimicking dlt output (with `_dlt_id`, `_dlt_load_id`) for engine round-trip tests.

## What M1 deliberately does NOT include

- **User-overridable dlt invariants** — locked in the runner. See [ADR-002](../../adr/0002-no-dlt-normalization.md).
- **Separate prism state file** — dlt + DuckDB suffice.
- **Auto-pruning of orphan tables/views** — when an entity contract is removed, its `__historized` table and `__current` view linger. Explicit `prism das prune` deferred to M4.
- **Schema-evolution migrations** — adding/removing/retyping a column in a contract requires the user to drop the historized table manually (or accept it; new rows have new schema). Automated migration is M4 territory.
- **Custom `latest_by:` ordering** — `__current` always orders by `_loaded_at DESC, _record_hash DESC`. Allowing a user-declared timestamp column for ordering is post-M1.
- **Multi-warehouse / concurrent runs** — DuckDB single-file exclusive lock; single-process assumption.
- **Auth frameworks** — first auth'd source (likely M2) forces the design.

## Known caveats

- **Hash stability after re-typing** — `_record_hash` is computed over the *typed* row struct, not the raw JSON. This is robust to upstream JSON-key-ordering changes (we don't rely on JSON byte stability). It does mean: if a contract's `type:` for a column changes (e.g., `INTEGER` → `BIGINT`), every existing row's hash changes, and the next build re-appends every row. Document; treat type changes as schema migrations.
- **DECIMAL precision** — declaring `type: DECIMAL` requires explicit precision/scale (`DECIMAL(18,4)`); generator rejects bare `DECIMAL`.
- **`uv.lock` commit policy** — gitignored by default in M1. Toggle is post-M1.
- **cgo cross-compilation** — `go-duckdb` requires a C toolchain at build time. CI uses `goreleaser` with cross-compilation; release binaries are pre-built per platform. `go install` from source needs the user's local C toolchain — fine for early adopters; document it.
- **Concurrent `prism run`** — DuckDB single-file is exclusive-lock. Two concurrent prism processes against the same warehouse error out. Expected behavior.

## Seams for M2

DAB will plug in by:

1. Adding a layer namespace — `contracts/dab/` alongside `contracts/das/`.
2. Reading from `das__<source>.<entity>__current` (or `__historized` for full history). Both are typed; DAB's mappings reference column names from the DAS contract (`target_name`), not JSON paths. The mapping syntax (descriptor projection, atomic-context grouping) is an M2 design question.
3. Extending the `Engine` and `Dialect` interfaces with DAB-specific operations (likely `MergeDescriptor`, `UpsertFocal`, hash-function abstraction).
4. Adding a layer to `prism run` — `das run --all → dab run --all`.

Drift handling between layers: a DAS contract change (added/renamed/retyped column) invalidates only the DAS staging tables for that entity. DAB mappings that reference removed/renamed `target_name` columns will fail validation, surfacing the impact. DAR consumers are unaffected unless DAB's surface changes.

Nothing in M1 forecloses anything M2 needs.
