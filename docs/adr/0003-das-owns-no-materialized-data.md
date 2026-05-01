# ADR-003: DAS has landing + typed staging; drift surfaces in DAS, not DAB

**Date:** 2026-05-01
**Status:** Accepted (revised mid-brainstorm; supersedes the initial draft of this ADR)
**Milestone:** M1

## Context

We initially proposed that DAS would materialize nothing — only `read_json_auto` views over the JSONL archive — on the theory that the file archive is the historization and that per-column schema at DAS would be redundant with DAB's descriptor declarations.

A later read of [The Rise of the Model-Driven Data Engineer](https://blog.daana.dev/blog/the-rise-of-model-driven-data-engineer) and reflection on operational concerns surfaced two problems with that design:

1. **Drift surfaces in the wrong place.** When a source renames `CustomerID` to `account_id`, a view-only DAS doesn't notice. The first failure shows up downstream in DAB, where it looks like a business-mapping bug — confusing to diagnose. The Daana article articulates the right shape: "Ingestion is forgiving (accept all data as JSON); unpacking is strict (contract-driven)." Drift should fail loudly at the layer that has a contract describing the source.

2. **DAB cannot be cheaply rebuilt.** With DAS materializing nothing, every DAB rebuild has to re-scan compressed JSONL files, re-infer types, and re-deduplicate from scratch. A typed, deduped, indexed surface at DAS makes DAB recomputation tractable.

The "redundancy" worry that drove the initial ADR was based on a category error: DAS schema and DAB descriptors describe **different things**, not the same thing twice.

| | DAS contract | DAB descriptor |
|---|---|---|
| Describes | Source-system shape | Business concept |
| Fields | `source_path`, `target_name`, `type`, `mode` | descriptor name, descriptor type (e.g. `START_TIMESTAMP`, `UNIT`), atomic context, bi-temporal flag |
| Question it answers | "Where in the JSON is this, and what was its type on the wire?" | "What does this attribute mean to the business?" |
| Drift it catches | Source moved/renamed a field, type changed | Business concept changed (rare, deliberate) |

## Decision

DAS has **two sub-stages**:

### 1. Landing (forgiving)

dlt's filesystem destination writes JSONL to `_lake/das/<source>/<entity>/`. No normalization beyond `_dlt_id` and `_dlt_load_id` (see [ADR-002](0002-no-dlt-normalization.md)). This is the immutable raw archive.

### 2. Staging (strict, contract-driven)

Each DAS entity contract carries per-column schema:

```yaml
schema:
  primary_key:
    - CustomerID
  columns:
    - source_path: CustomerID
      target_name: customer_id
      type: BIGINT
      mode: REQUIRED
      description: "Customer master ID"
```

`prism das build` generates SQL that produces, per entity:

| Object | Type | Purpose |
|---|---|---|
| `das__<source>.<entity>__historized` | typed **table**, append-on-hash | Typed audit log. Drift surfaces here (REQUIRED→NULL detection, cast failures). |
| `das__<source>.<entity>__current` | **view**: latest row per `primary_key` ordered by `_loaded_at DESC` | Stable typed surface for DAB to consume. |

The historized table's columns are: contract-declared columns (with types from the contract), plus `_record_hash` (PK), `_dlt_id`, `_dlt_load_id`, `_loaded_at`. Inserts are deduped by `_record_hash` (hash of typed values, excluding the metadata columns) using `ON CONFLICT DO NOTHING`.

The historized table is **the typed audit log**, complementing the JSONL archive (which remains the raw, pre-typing record).

## Consequences

**Positive:**

- Drift detection happens in DAS, where the contract describes the upstream contract. Failure modes are: cast errors (type changed), NULL in REQUIRED column (field renamed/removed), missing source_path (structure changed). All visible in `prism das build` output and in tier-1 contract tests.
- DAB rebuilds are cheap. DAB reads from `__current` (a view over indexed historized). No re-parsing of compressed JSONL on each DAB build.
- Schema redundancy between DAS and DAB is principled, not duplicative — different layers describe different things (see context table above).
- Three-layer stability: when a source renames a column, only the DAS contract changes; the DAB and DAR layers are insulated. (Daana: "Three-layer architecture provides stability.")
- Typed audit log enables point-in-time queries directly at DAS — useful for debugging and for DAB recomputation from a specific historical state.

**Negative:**

- Contracts grow. Per-entity files with full column declarations. AdventureWorks will produce ~70 files of ~10–30 columns each.
- Generator complexity: the SQL template library has to handle typed projection and casting from JSON paths. Larger than the view-only path.
- More DuckDB objects: 2 per entity × ~70 entities for AdventureWorks ≈ 140 objects. Cheap, but they exist.

**Mitigations:**

- `prism das discover` regains its scaffolding role: from upstream metadata (e.g. OData `$metadata`), it generates draft per-entity contracts. The user reviews and commits. Re-discover does drift detection against committed contracts.
- The type system is intentionally small (`STRING`, `INTEGER`, `BIGINT`, `DECIMAL`, `BOOLEAN`, `DATE`, `TIMESTAMP`, `JSON`). Generator complexity is bounded.

## Alternatives considered

**A. View-only DAS (initial draft of this ADR).** Materializes nothing. Simpler. Rejected because (i) drift surfaces in the wrong layer, (ii) DAB rebuilds are expensive without an indexed typed surface, (iii) the redundancy worry conflated DAS source-shape with DAB business-meaning.

**B. Typed `__current` only, no `__historized`.** Materialize the latest snapshot only; rely on JSONL archive for history. Rejected because rebuilding DAB from a specific historical state would require reprocessing JSONL through the typed-cast pipeline every time. The `__historized` table is the typed audit log; DAB can rebuild from it cheaply.

**C. Daana exact: typed `historized` + `latest`, change-tracking via a domain timestamp column.** Adopted in spirit. We default to `_loaded_at DESC` for the `__current` ordering, with contract-declared `latest_by:` override planned for post-M1.

## Related

- [ADR-002](0002-no-dlt-normalization.md) — landing's invariants. Staging is built on top of the faithful raw archive.
- [ADR-007](0007-one-yaml-per-source.md) — was revised in step with this ADR. Per-column schema requires per-entity files.
- Daana blog: [The Rise of the Model-Driven Data Engineer](https://blog.daana.dev/blog/the-rise-of-model-driven-data-engineer).
