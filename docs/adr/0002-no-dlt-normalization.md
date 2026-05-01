# ADR-002: dlt does no normalization; only `_dlt_id` and `_dlt_load_id` are added

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1

## Context

dlt's default behavior is aggressive normalization: it pivots nested arrays into child tables, snake-cases column names, infers and coerces types, and adds metadata columns. This is convenient for users who want a tabular result without thinking, but it is opinionated transformation — exactly what we want to push out of the landing layer and into DAB (where business semantics live).

If we accept dlt's defaults, the JSONL files in `_lake/` no longer reflect the source verbatim. Two consequences:

1. **DAB mappings get harder.** The user has to reason about both the source's actual structure *and* dlt's transformation of it. JSON path expressions in mappings have to track dlt's renaming rules.
2. **Schema drift surprises.** Provider adds a nested array → dlt pivots it into a new child table → suddenly we have unexpected files on disk and unexpected referenced tables.

We want the file archive to be a **faithful, byte-for-byte (modulo two metadata fields) record of what the source returned**, so DAB has a single, predictable surface to project from.

## Decision

The shared `prism_dlt_runner` (see ADR-006) **always** invokes dlt with the following invariants. They are **runner-enforced constants**, not exposed as YAML knobs:

```python
PRISM_INVARIANTS = dict(
    write_disposition  ="append",
    loader_file_format ="jsonl",
    max_table_nesting  =0,             # no child-table pivoting for nested arrays/objects
    naming_convention  ="direct",      # no snake_case rewriting
    add_dlt_id         =True,
    add_dlt_load_id    =True,
)
```

Result: each landed JSONL line is the source object verbatim, plus exactly two added keys: `_dlt_id` (per-row UUID) and `_dlt_load_id` (per-load batch ID).

If any of these invariants ever needs to change, that is a prism release decision, not a per-contract knob.

## Consequences

**Positive:**

- DAB sees the source's actual shape. Mappings reference paths that correspond 1:1 with what the API returned.
- File archive is auditable: a JSONL line in `_lake/` is the source object, plus identifiers, with no transformation between API and disk.
- No DAS-level normalization to drift from. New nested fields appear automatically; nothing pivots.
- Hash-based change detection in DAB (planned for M2) is meaningful: hashing the source object minus the two metadata keys yields stable, comparable values across re-lands.

**Negative:**

- Nested arrays/objects stay nested. DuckDB handles this fine via STRUCT/LIST types in `read_json_auto`, but DAB mappings need to navigate nested paths.
- Users coming from a default-dlt setup may expect snake_case column names. Documentation needs to call this out.
- A small subset of dlt features (incremental scopes that depend on inferred schema, deduplication via `merge` disposition) become unavailable in DAS. Acceptable: those features are DAB's job in our design.

## Alternatives considered

**A. Expose the invariants as `defaults.dlt` in `prism.yml`.** Allows users to relax them per-repo. Rejected because: (i) they are correctness invariants, not preferences — relaxing them breaks downstream contracts; (ii) once exposed, they accumulate adopters and become impossible to unset later.

**B. Accept dlt defaults.** Less code, easier onboarding for default-dlt users. Rejected because it puts source-specific transformation into the landing layer, defeating the point of layered architecture.

**C. Configure invariants per-provider in the runner's `providers/` modules.** Some flexibility, but invariants are about how prism uses dlt, not about the source. Best held global. Rejected.
