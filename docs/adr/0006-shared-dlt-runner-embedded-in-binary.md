# ADR-006: Shared dlt runner, embedded in the Go binary

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1

## Context

Prism orchestrates dlt via per-source uv venvs (ADR-001). Each `_pipelines/<source>/` venv needs *something* to invoke that knows how to read a prism source contract and run dlt with prism's invariants (ADR-002). Three patterns:

- **Shared runner** — one Python module, used by every source. Reads any source contract and dispatches by `provider:` to the right dlt source factory.
- **Codegen runner per source** — prism generates a `pipeline.py` per source, tailored to that source's provider. Each `_pipelines/<source>/` contains both `pyproject.toml` and `pipeline.py`.
- **Mixed** — shared base, with per-source overrides.

A shared runner needs to live somewhere users can run it from. Three sub-options:

- **Published Python package** (PyPI). Each `_pipelines/<source>/pyproject.toml` depends on `prism-dlt-runner==<version>`.
- **Embedded in the Go binary** via `go:embed`. Extracted on first use to a known cache path; each `_pipelines/<source>/pyproject.toml` references it via path-dependency.
- **Vendored separately** in the warehouse repo. User's repo includes a copy of the runner.

## Decision

**Shared runner.** One Python module — `prism_dlt_runner` — that reads any source contract and dispatches by `provider:`. No per-source codegen.

**Embedded in the Go binary** via `go:embed`. On first use, prism extracts the runner to `~/.cache/prism/dlt_runner/<prism-version>/`. Each `_pipelines/<source>/pyproject.toml` references the extracted path as a uv path dependency. Versioned by the prism release; never modified by users.

```
runtime/dlt_runner/                       # in prism Go repo, embedded into binary
├── __main__.py                           # entry point: uv run python -m prism_dlt_runner
├── runner.py                             # contract loading, dispatch, event emission
├── providers/
│   └── odata.py                          # builds dlt source for OData
│   # M2/M3: rest_api.py, sql_database.py, …
├── events.py                             # structured stdout emitter
└── pyproject.toml.tmpl                   # template for _pipelines/<source>/pyproject.toml
```

## Consequences

**Positive:**

- Patterns encoded once, in the runner — Daana's "consolidate knowledge into reusable patterns" principle, applied to extraction logic.
- Adding a new provider is a two-file PR-sized change: a new `providers/<name>.py` plus a dispatch entry. YAML schema doesn't change.
- Runner versioning is automatic — it ships with the prism release that embedded it. No risk of runner/CLI version mismatch in user environments.
- No PyPI publishing infrastructure needed.
- Users see no Python files in their warehouse repo. The runner is invisible to them.

**Negative:**

- The runner has to handle every provider via dispatch, which adds a small layer of indirection compared to direct factory imports.
- The extracted-cache-path strategy (`~/.cache/prism/dlt_runner/<version>/`) needs careful first-use semantics: extract atomically, handle stale cache cleanup, support read-only filesystems gracefully.
- If a user ever needs custom Python for a source, the answer is "we encode the pattern back into the runner" — not "drop in a one-off `pipeline.py`". This is a feature, not a bug, but worth being explicit about.

## Alternatives considered

**A. Codegen runner per source.** Each `_pipelines/<source>/pipeline.py` is generated from a template tailored to that source's provider. Debuggable per source; easier to drop in custom Python. Rejected because: (i) it duplicates patterns across sources, opposite of Daana's lesson; (ii) generated code in repos is a drift hazard (regeneration skipped → stale code); (iii) the "I need custom Python here" use case is an anti-pattern we want to prevent, not enable.

**B. Published PyPI package.** Each `_pipelines/<source>/pyproject.toml` depends on `prism-dlt-runner==<version>`. Cleaner separation, but: (i) requires PyPI publishing infrastructure for every prism release; (ii) decouples runner from CLI version, creating mismatch risk; (iii) users now have a Python package they can theoretically install/upgrade independently — surface area for support bugs. Rejected; revisit only if there's a strong reason to allow runner upgrades independent of prism.

**C. Vendored in the user's warehouse repo.** Reproducible, transparent. Rejected because the runner is engine code, not warehouse content. Vendoring creates noise in user repos and drift across users.

## Related

- [ADR-001](0001-single-go-binary-and-uv-venvs.md) — establishes the per-source uv venv pattern this ADR fills in.
- [ADR-002](0002-no-dlt-normalization.md) — invariants enforced by this runner.
