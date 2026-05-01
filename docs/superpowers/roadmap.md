# Prism — declarative data architecture roadmap

A fully declarative warehouse where the user writes only YAML contracts. The CLI (`prism`, written in Go) reads contracts and produces a working DuckDB warehouse end-to-end. Three layers, each its own milestone after M1, then orchestration.

## Influences

- Daana blog: [Contract-Driven Data Transformation](https://blog.daana.dev/blog/contract-driven-data-transformation) — the "contract is the documentation, schema, and transformation logic" framing.
- Daana docs: [DMDL](https://docs.daana.dev/docs/dmdl), [Focal Framework](https://docs.daana.dev/docs/concepts/focal-framework) — the four-pillar (Model / Mapping / Workflow / Connections) decomposition and the IDFR / Descriptor / Relation physical model.

We diverge from Daana in two ways: the CLI is Go (not Python), and DAS contracts carry no per-column schema — typing-with-business-meaning is paid once at DAB.

## Architectural decisions

Each foundational decision has its own ADR under [docs/adr/](../adr/). Current set:

| ADR | Decision |
|---|---|
| [001](../adr/0001-single-go-binary-and-uv-venvs.md) | Single Go binary; per-source uv venvs for dlt |
| [002](../adr/0002-no-dlt-normalization.md) | dlt does no normalization; only `_dlt_id` and `_dlt_load_id` are added |
| [003](../adr/0003-das-owns-no-materialized-data.md) | DAS owns no materialized data; the JSONL archive is the historization |
| [004](../adr/0004-go-duckdb-over-adbc.md) | go-duckdb (cgo) for DuckDB; Engine interface for future engines |
| [005](../adr/0005-double-underscore-as-concern-separator.md) | `__` is the single concern separator in DuckDB names |
| [006](../adr/0006-shared-dlt-runner-embedded-in-binary.md) | Shared dlt runner, embedded in the Go binary |
| [007](../adr/0007-one-yaml-per-source.md) | One YAML file per source; source ID derived from filename |

New milestones add ADRs as new decisions arise. Existing ADRs are amended via Status (Superseded by ADR-NNN), not edited in place.

## Architectural invariants (apply to all milestones)

| Invariant | Rationale |
|---|---|
| **One Go binary** (`prism`) is the only thing the user installs aside from `uv`. | Single distribution; no Python visible to users beyond uv venv management. |
| **All user input is YAML.** No hand-written SQL or Python anywhere in the warehouse repo. | Strict declarative. |
| **Per-source uv venvs** under `_pipelines/<source>/`. | Each source pins its own dlt extras; no cross-source dependency conflicts. |
| **Shared dlt runner**, embedded in the Go binary via `go:embed`. | Daana pattern: encode patterns once in the engine, never duplicate per pipeline. |
| **dlt does no normalization.** Only `_dlt_id` and `_dlt_load_id` are added; `max_table_nesting=0`, `naming_convention=direct`. These are runner invariants, not YAML knobs. | Source bytes preserved verbatim through to DAB; no provider-specific surprises. |
| **`__` is the single concern separator** in DuckDB names. Schemas: `<layer>__<source>`. Objects: `<entity>__<role>`. | Consistent, parseable, never collides with snake_case identifiers. |
| **`Engine` interface from day 1.** Only DuckDB implementation in M1; future engines (Postgres, Databricks via Flight SQL, BigQuery, Fabric) plug in here, possibly via ADBC. | Real portability work is SQL-dialect, not connection protocol — but the interface keeps DuckDB-specific code firewalled. |
| **The JSONL archive in `_lake/` IS the historization.** No materialized historized table at DAS. | Avoids a parallel source of truth. dlt's filesystem destination is designed as the archive. |
| **No prism-managed state file.** dlt owns incremental cursors; DuckDB holds materialized layers. | Two stateful surfaces, each owned by a tool that already does it well. |

## Naming conventions

| What | Pattern | Example |
|---|---|---|
| Source ID | snake_case, derived from filename | `adventure_works.yml` → `adventure_works` |
| Entity name in DuckDB | snake_case, auto-converted from source | `Customer` → `customer`, `SalesOrderHeader` → `sales_order_header` |
| Schema (per layer × source) | `<layer>__<source>` | `das__adventure_works`, `dab__adventure_works` |
| Object suffix | `<entity>__<role>` | `customer__stage`, `customer__address` (descriptor), `customer__order__rel` (relation) |
| DAR schema | `dar` (single, unified across sources) | `dar.bridge`, `dar.customer__dim`, `dar.sales__fact` |

## Repository layout (target)

```
declarative-data-architecture/
├── contracts/
│   ├── das/<source>.yml                  # M1
│   ├── dab/<concept>.yml                 # M2
│   └── dar/<...>.yml                     # M3
├── _lake/                                # gitignored — dlt filesystem destination
├── _pipelines/                           # gitignored — uv-managed venvs
├── warehouse.duckdb                      # gitignored — DuckDB single-file
├── prism.yml                             # optional repo config
└── docs/superpowers/
    ├── roadmap.md                        # this file
    └── specs/                            # one per milestone
```

The `prism` Go source lives in a separate repo. This repo holds only contracts and outputs.

## Milestones

### M1 — DAS (Data According to System)

**Status:** designed in `specs/2026-05-01-prism-m1-das-design.md`.

**Deliverable:** working `prism` binary that turns `contracts/das/<source>.yml` into a queryable DuckDB façade over a JSONL archive. AdventureWorks OData is the validation dataset.

**Surface:** `prism init`, `prism validate`, `prism doctor`, `prism das discover|land|build|run [<source>] [--all]`, `prism run`.

**DuckDB output:** `das__<source>.<entity>__stage` views (`read_json_auto` over JSONL). No materialization. No per-column schema declared.

**Key constraint:** typing and business semantics are deferred to M2 — DAS only knows what to land and where.

### M2 — DAB (Data According to Business)

**Status:** roadmap-only. Brainstorm at start of M2.

**Deliverable:** focal-framework physical model generated from concept contracts. Implements Daana's IDFR / Descriptor / Relation tables with bi-temporal columns (`EFF_TMSTP`, `VER_TMSTP`, `SEQ_NBR`, `ROW_ST`).

**New contract layer:** `contracts/dab/`. Includes both:
- **Model declarations** — focals, descriptors, relations, atomic context (DMDL Model pillar).
- **Mappings** — projection from `das__<source>.<entity>__stage` columns into descriptor attributes / focal IDFRs (DMDL Mapping pillar). The exact mapping syntax (column refs vs. JSON paths vs. STRUCT navigation) is part of the M2 design.

**DuckDB output:** `dab__<source>.<entity>` (focal IDFR), `dab__<source>.<entity>__<descriptor>`, `dab__<source>.<entity>__<related>__rel`.

**Key design questions to resolve at M2 brainstorm:**
- Mapping syntax — projecting from stage views (typed scalars + STRUCT/LIST for nested) into descriptors.
- Where does cross-source identity reconciliation live (e.g., the same Customer in two systems)? In DAB or DAR?
- How are `TYPE_KEY` registries declared? Inline per descriptor or in a separate `contracts/dab/_types.yml`?
- Hash canonicalization for change detection — JSON key ordering, type coercion order, NULL handling.
- DAB-only `prism dab discover` — does it surface what DAS exposes, to scaffold mappings?
- Engine interface extensions: `MergeDescriptor`, `UpsertFocal`, dialect-specific hash functions.

### M3 — DAR (Data According to Requirements)

**Status:** roadmap-only. Brainstorm at start of M3.

**Deliverable:** unified Puppini-bridge star schema. Single `dar` schema. One bridge fact, conformed dimensions per business concept, declarative measures.

**New contract layer:** `contracts/dar/`. Likely:
- **Dimensions** — declarative selection of which DAB descriptors form a conformed dimension.
- **Bridge** — declarative selection of which focals participate, and grain.
- **Measures** — declarative aggregations on bridge / dimension joins.

**DuckDB output:** `dar.bridge`, `dar.<concept>__dim`, `dar.<grain>__fact`.

**Key design questions to resolve at M3 brainstorm:**
- Bridge cardinality / grain handling.
- Slowly-changing-dimension policy from DAB descriptors.
- Whether measures are declarative SQL or a more constrained DSL.
- Late-arriving facts.

### M4 — Orchestration & polish

**Status:** roadmap-only. Brainstorm at start of M4.

**Deliverable:** `prism` becomes feature-complete for day-to-day operation.

**Likely scope:**
- `prism run` runs DAS → DAB → DAR end-to-end.
- Freshness checks (`prism freshness`).
- Lineage view derived from contracts (`prism lineage [<object>]`).
- `prism das prune` — drop orphan stage views for entities removed from contracts.
- Configurable JSONL retention policy.
- Optional `uv.lock` commit pattern toggle.
- Concurrent-run safety review (DuckDB exclusive lock).
- Release engineering: `goreleaser` cross-build with cgo for go-duckdb.

## Out of scope (for now, all milestones)

- Multiple warehouse files / multi-tenant DuckDB.
- Streaming / push ingestion (dlt is pull-only).
- Server / API mode (prism is a CLI).
- Authentication frameworks beyond what `provider:` blocks declare.
- Dbt / SQLMesh integration — prism is the engine.

## How a future milestone is started

1. Open this roadmap; lift the milestone's section.
2. Run `superpowers:brainstorming` against that scope; produce a spec at `docs/superpowers/specs/YYYY-MM-DD-prism-m<n>-<layer>-design.md`.
3. Run `superpowers:writing-plans` to produce the implementation plan.
4. Update this roadmap with what was actually decided (status flips, scope sharpens).
