# ADR-001: Single Go binary for the CLI; per-source uv venvs for dlt

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1

## Context

Prism is a declarative data warehouse. Users author YAML contracts; a CLI reads them and produces a working DuckDB warehouse end-to-end. Two implementation languages are in tension:

- **Python** is the natural language for data tooling (dlt, dbt, sqlmesh, dagster). Most data engineers have a Python toolchain. dlt is Python-only.
- **Go** distributes as a single binary, has no runtime, ships fast for end users, and is well suited to file/IO/subprocess orchestration.

The user's stated preference is Go for the CLI. dlt cannot run outside Python. Different sources need different dlt extras (`dlt[odata]`, `dlt[sql_database]`, etc.) that may pin different transitive versions.

## Decision

The `prism` CLI is **a single Go binary**. Users install it from GitHub Releases (or `go install` if they have a C toolchain — see ADR-004).

dlt runs inside **per-source uv-managed Python venvs** under `_pipelines/<source>/`. Each venv pins its own dlt extras. Prism shells out to `uv run …` to invoke a shared Python runner (see ADR-006) inside each venv.

The user's only required external dependency beyond `prism` itself is `uv`.

## Consequences

**Positive:**

- Single binary distribution; no Python tooling visible to users beyond `uv`.
- Per-source venv isolation prevents dependency conflicts between sources (e.g., `dlt[postgres]` pins on a `psycopg` version that conflicts with another source's `dlt[mysql]`).
- Each `_pipelines/<source>/` is regenerable from contracts; can be gitignored. Reproducibility (committed `uv.lock`) is a future toggle.
- Go's `text/template`, `database/sql`, and subprocess primitives are well-suited to what prism does.
- Clear language boundary: Go owns orchestration, SQL generation, and DuckDB. Python (in venvs) owns dlt.

**Negative:**

- Cgo is required for DuckDB binding (see ADR-004). Cross-compilation needs CI infrastructure (`goreleaser` + zig).
- Two-language codebase. Python runner is a small surface (~one module, see ADR-006), but adds cognitive overhead.
- Subprocess + JSONL IPC has more moving parts than calling dlt as a library would.

**Mitigations:**

- Python surface is minimized (single shared runner, embedded in the Go binary, see ADR-006). Users never see Python source files in their warehouse repo.
- IPC format is well-defined JSONL events; contract-tested per release.
- Cgo cost is paid by maintainers in CI; users get pre-built binaries.

## Alternatives considered

**A. Pure Python CLI, dlt as library.** Single language. Loses Go's distribution simplicity. User sees Python everywhere. Dependency-conflict risk if multiple sources need different dlt extras in the same env. Rejected because the user explicitly wanted Go.

**B. Pure Go, no dlt.** Reimplement extraction (REST, OData, SQL) in Go. Years of work, reinvents what dlt already does well. Rejected.

**C. Codegen Python per pipeline (like dbt generates SQL).** Each `_pipelines/<source>/` contains a generated `pipeline.py`. Debuggable per pipeline, but multiplies the surface area, defeats "patterns encoded once" (see Daana blog). Rejected in favor of shared runner (ADR-006).
