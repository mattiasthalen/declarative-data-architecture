# ADR-007: One YAML per entity; one `_source.yml` per source

**Date:** 2026-05-01
**Status:** Accepted (revised mid-brainstorm; supersedes the initial draft of this ADR)
**Milestone:** M1 (sets convention for all milestones)

## Context

A DAS source declaration carries: provider config (URL, auth, etc.) and a list of entities, each with per-column schema (source_path, target_name, type, mode — see [ADR-003](0003-das-owns-no-materialized-data.md)). We need a file layout for `contracts/das/`. Two options remain after ADR-003:

- **One file per source, all entities inline.** Single-file convenience. With ~70 entities × 10–30 columns, AdventureWorks produces a single ~2,000-line file. Editing, diffing, and reviewing a single concept in that file is hard.
- **One file per entity, plus a per-source file for provider config.** Daana's pattern (their blog uses one contract per entity). Files are small, focused, and reviewable.

The initial draft of this ADR chose one-file-per-source on the assumption that DAS contracts would carry no per-column schema. ADR-003 revised that assumption: per-column schema is now part of the DAS contract. Single-file-per-source no longer works at scale.

## Decision

**One YAML per entity.** Plus exactly one `_source.yml` per source for provider configuration. Layout:

```
contracts/
└── das/
    └── adventure_works/
        ├── _source.yml           # provider, base_url, auth
        ├── customer.yml          # per-entity contract with columns
        ├── product.yml
        ├── sales_order_header.yml
        └── …
```

**Source ID derives from the directory name** under `contracts/das/`. `contracts/das/adventure_works/` → source ID `adventure_works`.

**Entity ID derives from the filename basename** within the source directory. `customer.yml` → entity ID `customer` → DuckDB tables `das__adventure_works.customer__historized` and `das__adventure_works.customer__current`.

**Filename rules** (validation enforces):

- Source directory and entity filename basenames are snake_case.
- `_source.yml` is reserved (the leading underscore distinguishes config from entity files).
- No subdirectories below the source directory in M1.

**Entity file structure:**

```yaml
# contracts/das/adventure_works/customer.yml
version: 1
entity:
  name: Customer                  # name as upstream exposes it (e.g., OData entity set)
incremental:                      # optional
  cursor: ModifiedDate
  strategy: append
schema:
  primary_key:
    - CustomerID
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
```

`entity.name` is the upstream-system name (used by dlt to extract). The DuckDB-side name comes from the filename (snake_case basename), independently. This lets us keep the upstream's casing for extraction while having clean snake_case identifiers in the warehouse.

**`_source.yml` structure:**

```yaml
# contracts/das/adventure_works/_source.yml
version: 1
source:
  provider: odata
  base_url: https://demodata.grapecity.com/adventureworks/odata/v1/
```

## Consequences

**Positive:**

- Each entity is a focused, reviewable file. Adding a column is a one-line edit in the right file; renaming an entity is a `git mv`.
- `prism das discover` can write per-entity scaffolds idempotently — generating one file per discovered entity, leaving any existing files alone unless `--update` is passed.
- Drift detection per-entity is natural: `prism validate` and `prism das build` can produce per-file errors, not "line 1247 of source.yml".
- Convention extends cleanly: M2 will likely follow `contracts/dab/<concept>/<thing>.yml`, M3 follows `contracts/dar/<thing>.yml`.

**Negative:**

- File proliferation. AdventureWorks produces ~70 entity files plus 1 source file. Manageable, but it's many files. Modern editors and `git` handle this fine; humans skimming a directory listing don't.
- Two filename conventions in the same directory (`_source.yml` reserved, `<entity>.yml` for entities). Slight cognitive load; mitigated by the leading-underscore convention.
- Source-level config and entity-level config live in separate files. Cross-cutting changes touch multiple files. Acceptable: cross-cutting changes are rare; per-entity edits are common.

## Alternatives considered

**A. One YAML per source, all entities inline.** Initial draft of this ADR. With per-column schema (ADR-003), files become unmanageably large. Rejected.

**B. Subdirectories per entity** (`contracts/das/adventure_works/customer/{schema,incremental}.yml`). Over-decomposed for M1. Rejected.

**C. Source ID and entity name in the YAML body** (a `source: <id>` and `entity: <id>` field, with filename free-form). Adds redundancy and a class of validation errors ("filename doesn't match `id:` field"). Filename-derived is cleaner; rename = `git mv`. Rejected.

**D. PascalCase entity files** (`Customer.yml`). Inconsistent with the warehouse-side snake_case convention (ADR-005). Rejected.

## Related

- [ADR-003](0003-das-owns-no-materialized-data.md) — per-column schema requires per-entity files; the ADRs were revised together.
- [ADR-005](0005-double-underscore-as-concern-separator.md) — naming convention for the resulting DuckDB objects.
