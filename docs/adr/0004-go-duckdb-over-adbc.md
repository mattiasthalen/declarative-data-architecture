# ADR-004: go-duckdb (cgo) for DuckDB; Engine interface for future engines

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1

## Context

Prism is a Go binary that needs to talk to DuckDB. Three viable bindings:

- **`go-duckdb`** — cgo binding to DuckDB's C library, exposed via Go's `database/sql`. Native row iteration, prepared statements, structured errors.
- **DuckDB CLI subprocess** — generate SQL, shell out to `duckdb warehouse.duckdb -c "…"`, parse stdout. Pure-Go, no cgo.
- **ADBC** (Apache Arrow DB Connectivity) — Arrow-native standard. DuckDB ships an ADBC driver. Multiple engines (Postgres, BigQuery, Snowflake, Flight SQL) also expose ADBC.

The user raised future-proofing as a concern: "If we want to target Fabric / Databricks / Postgres / BigQuery later, isn't ADBC the more future-proof choice?"

This deserves a careful answer. ADBC standardizes the *connection protocol*, not the *SQL dialect*. The portability cost of supporting another engine is dominated by:

| Concern | DuckDB | Postgres | BigQuery | Databricks | Fabric |
|---|---|---|---|---|---|
| Read JSON files | `read_json_auto` over local glob | none — must `COPY` from staged file | external table over GCS | autoloader from S3/ADLS | OPENROWSET over OneLake |
| Nested types | STRUCT/LIST | jsonb | STRUCT/ARRAY | STRUCT/ARRAY | JSON column |
| Hash function | `md5()` | `md5()` | `MD5()` returning bytes | `md5()` | `HASHBYTES('MD5', …)` |

Connection protocol is ~5% of porting work; SQL templates and file-staging are the other 95%. Choosing ADBC for DuckDB now buys no portability benefit, since DuckDB's ADBC driver is also cgo and the dialect work is unchanged.

## Decision

For DuckDB in M1: **`go-duckdb` via `database/sql`**.

For future engines (post-M1): **a small `Engine` interface** behind which all engine-specific code lives. Implemented from day 1, but only with a DuckDB implementation. When a second engine is added (M5+), it provides its own `Engine` and `Dialect` implementations, choosing whichever connection driver is most practical for that engine — could be ADBC for some, native drivers for others.

```go
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
```

The interface starts minimal and grows per layer. DAS adds only `CreateSchemaIfNotExists` and `CreateOrReplaceStageView`. DAB and DAR will add their own methods (likely `MergeDescriptor`, `UpsertFocal`, hash-function abstraction).

## Consequences

**Positive:**

- Best DuckDB ergonomics for M1: type-safe row scanning, prepared statements, structured errors via standard `database/sql`.
- Engine interface keeps DuckDB-specific code firewalled. Future engines plug in cleanly.
- Driver choice is per-engine when we add them — no premature lock-in to ADBC across the board.
- M1 ships fast, with the right primitive (interface) in place for the right reasons.

**Negative:**

- cgo: cross-compilation needs a C toolchain for each target platform. CI uses `goreleaser` with cross-build (zig as cross-compiler) — solvable, but real release-engineering work.
- Users running `go install` from source need a local C toolchain. Most users will install pre-built release binaries; document the source-install requirement.
- Slightly larger binary than a pure-Go alternative.

**Mitigations:**

- Release binaries via GitHub Releases are the primary install path (mirroring `gh`, `lazygit`, etc.). `go install` is documented as a power-user path.
- `goreleaser` config is a one-time cost; reusable per release.

## Alternatives considered

**B. DuckDB CLI subprocess.** Pure-Go binary, `go install` works without cgo. But: requires users to install the `duckdb` CLI separately (or vendor a binary per platform — back to release-engineering complexity); stringy IPC; no streaming; awkward error handling. The driver-friction trade is in roughly the same place — we'd be solving the same release-engineering problems either way. Rejected.

**C. ADBC.** Cgo too. ADBC's Go ecosystem is less mature than `go-duckdb`. Provides no cross-engine benefit at the dialect level, which is the actual portability bottleneck. Rejected for M1; remains a viable choice for individual future engines behind the `Engine` interface.

## Related

- Roadmap "Engine interface from day 1" invariant.
- Future ADRs may revisit: when adding the second engine (M5+), a per-engine ADR will document driver choice for that engine.
