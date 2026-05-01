# ADR-005: `__` is the single concern separator in DuckDB names

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1 (sets convention for all milestones)

## Context

Prism generates many DuckDB schemas, tables, and views across three layers (DAS, DAB, DAR) and many sources. Names need to encode multiple concerns: layer, source, entity, role (stage / descriptor / relation / dim / fact / etc.). The naming scheme has to be:

- Parseable (a tool or human can identify the components from the name)
- Collision-free with snake_case identifiers (entity names like `sales_order_header` already contain underscores)
- Consistent across layers (so the rules learned in M1 still apply in M2 and M3)
- Compatible with DuckDB identifier rules (no quoting required for the common case)

Single-underscore separators (e.g., `das_adventure_works_customer`) collide with snake_case content. Hyphens require quoting. Dots (`.`) are reserved for the schema/object boundary.

## Decision

**`__` (double underscore) is the single separator between conceptual segments** in all generated DuckDB names. Single underscores are reserved for snake_case within a segment.

| Position | Pattern | Example |
|---|---|---|
| Schema | `<layer>__<source>` | `das__adventure_works`, `dab__adventure_works` |
| Object in schema | `<entity>__<role>` (when role exists) or `<entity>` (when canonical) | `customer__historized` (table), `customer__current` (view), `customer__address` (DAB descriptor), `customer__order__rel` (DAB relation) |
| DAR schema | `dar` (single, unified across sources) | `dar.bridge`, `dar.customer__dim`, `dar.sales__fact` |

Source IDs and entity names are snake_case (single underscore) within a segment; multi-segment composition uses `__`.

Source IDs **derive from the source contract filename** (e.g., `adventure_works.yml` → `adventure_works`). Entity names from upstream are auto-converted to snake_case (e.g., OData's `SalesOrderHeader` → `sales_order_header`).

## Consequences

**Positive:**

- Names are unambiguously parseable: split on `__` to extract concerns, then `_` is "just text" within a segment.
- No quoting required (`__` is valid in unquoted SQL identifiers).
- Convention scales across all milestones — DAB and DAR follow the same rule.
- File-naming is consistent (`adventure_works.yml` → snake_case basename → snake_case schema component).

**Negative:**

- `__` is slightly noisier visually than `_`. Names like `dab__adventure_works.customer__order__rel` are long.
- Tooling that case-folds or compresses underscores (some BI tools, some ORMs) may collapse `__` to `_` and break referential integrity for users querying through such tools. Document; revisit if it bites.

## Alternatives considered

**A. Single `_` as separator.** Collides with snake_case content. Rejected.

**B. Dots within table names** (e.g., `das.adventure_works.customer.stage`). Conflicts with SQL's schema/object boundary. Most engines reject identifiers with dots without quoting. Rejected.

**C. Mixed naming per layer.** Different layers use different separators. Rejected — consistency across milestones is more valuable than per-layer brevity.

**D. PascalCase / camelCase.** Many DuckDB / SQL tools fold case in queries (`Customer` and `customer` may collide depending on quoting). Rejected.
