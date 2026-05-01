# ADR-003: DAS owns no materialized data; the JSONL archive is the historization

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1

## Context

The Daana blog post on contract-driven transformation describes a DAS pattern with two materialized models per source entity: a `historized` (append-only, hash-deduped) table and a `latest` (deduped current) view. Their historized table has typed columns extracted from JSON, with `_record_hash` for dedup.

We initially proposed mirroring this pattern. Two design decisions caused us to reconsider:

1. **No per-column schema in DAS** — the user's "redundant schema definitions" pushback. Schema-with-business-meaning lives at DAB (focal-framework descriptors). Re-declaring typed columns at DAS to populate a historized table is exactly the redundancy we want to avoid.
2. **The JSONL files in `_lake/` are already a complete archive.** dlt's filesystem destination writes new files per load, never overwrites. For incremental sources, the union of files is a complete change log; for full-refresh sources, it's every snapshot ever taken. Materializing those into DuckDB creates a parallel source of truth that can drift from the file archive (e.g., if a user deletes JSONL files manually).

Two surfaces holding the same data is a maintenance burden with no clear win once typing is deferred to DAB.

## Decision

**DAS materializes nothing in DuckDB.** The only DuckDB objects DAS produces are **views**: one per (source, entity), defined as `read_json_auto` over the JSONL glob.

The historization story is:

| Layer | Form | Owner |
|---|---|---|
| **Archive** (every observation, ever) | Compressed JSONL files in `_lake/` | dlt's filesystem destination |
| **Stage** (queryable façade) | Views over the archive | DAS |
| **Bi-temporal projections** (typed, deduped, business-meaningful) | Tables: descriptors with `EFF_TMSTP` / `VER_TMSTP` / `SEQ_NBR`, focal IDFRs | DAB |

DAB consumes from `das__<source>.<entity>__stage` (which scans JSONL on every query). When DAB needs performance or deduped access, it materializes its own descriptor tables — which it does anyway as part of the focal framework.

## Consequences

**Positive:**

- Single source of truth for raw history: the file archive on disk.
- DAS contracts are minimal — provider, base URL, list of entities. No per-column toil.
- Prism keeps no state file; DuckDB views are derived purely from contracts on every build, dlt owns its incremental cursors. Two stateful surfaces, each owned by a tool already designed for it.
- Re-running `prism das build` is always safe — no migration logic, no historization to reconcile.
- Schema drift in DAS is automatic: `union_by_name = true` lets new upstream columns appear in the stage view without intervention.

**Negative:**

- Stage view query cost scales with archive size. Every DAB build that scans a stage view pays the JSONL parse cost. Acceptable for AdventureWorks; for very large sources, DAB's materialization is the answer.
- No queryable PIT structure between landed JSONL and DAB tables — point-in-time history lives in JSONL files, queried via `_dlt_load_id` filtering on the stage view. Less ergonomic than a typed historized table, but adequate.
- Hash-based dedup must happen in DAB. The DAB layer will hash projected descriptor values, not raw landed rows. (Hash strategy is a known M2 caveat — see roadmap "JSON key-ordering stability".)

## Alternatives considered

**A. Daana-blog model: historized + current per entity, with typed columns.** Requires per-column DAS schema. Redundant with DAB's typed descriptors. Rejected.

**B. Schema-agnostic historized: `_raw` JSON column + `_record_hash` + `_dlt_id` + `_dlt_load_id`.** No typed columns, but a materialized append-only table. Provides indexed dedup surface for DAB. Rejected because: (i) duplicates the file archive's content, creating a second source of truth; (ii) DAB's descriptor materialization already provides a dedup surface; (iii) every full-refresh source bloats the table with re-deduped snapshots over time.

**C. No DAS-level DuckDB objects at all.** DAB inlines the `read_json_auto` calls. Rejected because the named view provides a stable, debuggable interface (`SELECT * FROM das__... LIMIT 5` for inspection) and centralizes lake-path resolution.

## Related

- [ADR-002](0002-no-dlt-normalization.md) — without normalization, the JSONL archive is a faithful record, which makes "JSONL is the archive" workable.
- Roadmap §M2 — hash canonicalization and bi-temporal columns are DAB's job.
