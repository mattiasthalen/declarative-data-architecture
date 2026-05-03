# Prism M2 — DAB layer design

**Status:** Approved (brainstorm 2026-05-02).
**Roadmap context:** [docs/superpowers/roadmap.md](../roadmap.md).
**Decision records:** [docs/adr/](../../adr/).
**Predecessor:** [specs/2026-05-01-prism-m1-das-design.md](2026-05-01-prism-m1-das-design.md).

## Goal

Ship the DAB (Data According to Business) layer: a focal-framework physical model, generated from declarative `contracts/dab/<entity>.yml` files, that turns the typed DAS staging tables produced in M1 into an IDFR / Focal / Descriptor / Relationship physical model with full bi-temporal columns. DAB is **conformed across sources** — a single `dab` schema where one business entity (e.g. `CUSTOMER`) can absorb rows from multiple DAS sources.

The contract shape follows Daana's DMDL ([Model](https://docs.daana.dev/docs/dmdl/model), [Mapping](https://docs.daana.dev/docs/dmdl/mapping)) trimmed for prism's narrower context (DAS is the only upstream connection). Physical layout follows Daana's [Focal Framework](https://docs.daana.dev/docs/concepts/focal-framework). Bi-temporal columns and surrogate keys are mechanically derived; users never write `_key`, `EFF_TMSTP`, or `TYPE_KEY` values by hand.

## Success criteria

A clean clone of a warehouse repo with one DAS source (M1) and one DAB focal contract runs end-to-end:

```bash
prism init                               # M1
# author contracts/das/adventure_works/customer.yml          (M1)
prism das run adventure_works            # M1: lands + builds DAS
# author contracts/dab/customer.yml                          (M2)
prism dab build customer
duckdb warehouse.duckdb -c "FROM dab.customer__current LIMIT 5;"
```

…and yields rows where each customer business entity appears once, with descriptors filled from typed DAS columns and bi-temporal columns populated automatically.

A second DAS source (e.g. `stripe`) plugged into the same `customer.yml` via a second `mapping_groups[]` entry produces:

```sql
SELECT customer_idfr, inst_row_key FROM dab.customer__idfr WHERE customer_key = '…';
-- two rows: one per contributing DAS source/entity, sharing the same customer_key surrogate
SELECT * FROM dab.customer__current WHERE customer_key = '…';
-- one row, descriptors merged, ROW_ST = 'Y'
```

…with no state file added, idempotent on re-runs.

## Architecture

DAB is a **build-only** layer in M2. It reads typed DAS staging objects (`das__<source>.<entity>__current` or `__historized`) inside the same DuckDB file and writes IDFR / Focal / Descriptor / Relationship tables under a single `dab` schema. There is no land step — DAS already landed the JSONL.

```
                ┌──────────────────────────────────────┐
                │     User-edited (git-tracked)        │
                ├──────────────────────────────────────┤
                │  contracts/dab/<entity>.yml          │
                │     entity:        (model)           │
                │     attributes:    (model)           │
                │     relationships: (model)           │
                │     mapping_groups:  (mappings)      │
                │       - tables: [DAS source/entity]  │
                └──────────────┬───────────────────────┘
                               │
                ┌──────────────▼───────────────────┐
                │           prism (Go)             │
                │  ┌─────────────────────────────┐ │
                │  │  YAML loader + validator    │ │
                │  │  Engine interface (M1+M2)   │ │
                │  │  SQL template library       │ │
                │  │   └─ DAB DDL/merge/views    │ │
                │  └─────────────────────────────┘ │
                └────────────┬─────────────────────┘
                             │ go-duckdb (cgo)
                             ▼
        ┌──────────────────────────────────────────────────────────┐
        │                 warehouse.duckdb                          │
        │                                                           │
        │  schema  das__adventure_works            (M1)             │
        │   ├─ customer__historized                                 │
        │   └─ customer__current                                    │
        │                                                           │
        │  schema  das__stripe                     (M1, optional)   │
        │   └─ customer__current                                    │
        │                                                           │
        │  schema  dab                             (M2)             │
        │   ├─ customer__idfr             (IDFR table)              │
        │   ├─ customer                   (FOCAL table)             │
        │   ├─ customer__descriptor       (generic EAV descriptor)  │
        │   ├─ customer__order__rel       (relationship table)      │
        │   ├─ customer__customer_name    (typed view, single attr) │
        │   ├─ customer__customer_lifetime_value (typed view, group)│
        │   └─ customer__current          (focal+ROW_ST='Y' join)   │
        └──────────────────────────────────────────────────────────┘
```

**Three moving parts in M2** (additive to M1):

1. **`contracts/dab/`** — one YAML file per focal entity. Declares the model (entity + attributes + relationships) and the mappings (which DAS sources feed it).
2. **DAB build pipeline** — `prism dab build` reads contracts, walks DAS staging tables in the same DuckDB, and applies idempotent SQL to populate `dab.*` tables.
3. **Typed views** — generated alongside the EAV descriptor table to give analysts native-typed access to atomic-context groups.

See [ADR-003](../../adr/0003-das-owns-no-materialized-data.md) for why DAB is materialized while DAS is not (DAS owns no materialized data; DAB does).

## Repository layout

```
declarative-data-architecture/
├── contracts/
│   ├── das/                              # M1
│   │   └── adventure_works/
│   │       ├── _source.yml
│   │       └── customer.yml
│   └── dab/                              # M2 — single directory, no source split
│       ├── customer.yml
│       ├── product.yml
│       └── order.yml
├── _lake/                                # M1 — landed JSONL
├── _pipelines/                           # M1 — uv venvs
├── warehouse.duckdb                      # M1 + M2
├── prism.yml
└── docs/
    ├── adr/
    └── superpowers/
        ├── roadmap.md
        └── specs/
            ├── 2026-05-01-prism-m1-das-design.md
            └── 2026-05-02-prism-m2-dab-design.md
```

`contracts/dab/` is **flat, not source-partitioned** — entities are conformed business concepts, not source artifacts. A single `customer.yml` can pull from `das__adventure_works.customer__current`, `das__stripe.customer__current`, etc.

## YAML shape

### `contracts/dab/<entity>.yml`

One file per focal. Two sections: **model** (the business shape) and **mapping_groups** (how DAS sources project into it).

```yaml
# contracts/dab/customer.yml
version: 1

entity:
  id: CUSTOMER                          # UPPER_SNAKE; entity surrogate is hash of canonical IDFR
  name: CUSTOMER
  definition: "A customer account"
  description: "An individual or organization that purchases products."

attributes:
  # simple (single-attribute group)
  - id: CUSTOMER_NAME
    definition: "Display name of the customer"
    type: STRING
    effective_timestamp: true

  # atomic-context group (UNIT-coupled monetary value)
  - id: CUSTOMER_LIFETIME_VALUE
    definition: "Total monetary value the customer has produced to date"
    effective_timestamp: true
    group:
      - id: AMOUNT
        type: NUMBER
      - id: CURRENCY
        type: UNIT

  # period (START_TIMESTAMP / END_TIMESTAMP couple)
  - id: CUSTOMER_ACTIVE_WINDOW
    definition: "Window during which the customer was active"
    effective_timestamp: true
    group:
      - id: START
        type: START_TIMESTAMP
      - id: END
        type: END_TIMESTAMP

relationships:
  - id: CUSTOMER_PLACES_ORDER
    definition: "The customer placed this order"
    target_entity_id: ORDER

mapping_groups:
  - name: adventure_works
    allow_multiple_identifiers: false
    tables:
      - source: adventure_works         # DAS source ID (directory name)
        entity: customer                # DAS entity ID (file basename)
        from: current                   # 'current' (default) or 'historized'
        primary_keys:
          - "'CUSTOMER:' || CAST(customer_id AS VARCHAR)"
        entity_effective_timestamp_expression: modified_date
        attributes:
          - id: CUSTOMER_NAME
            transformation_expression: company_name
          - id: CUSTOMER_LIFETIME_VALUE.AMOUNT
            transformation_expression: lifetime_value
          - id: CUSTOMER_LIFETIME_VALUE.CURRENCY
            transformation_expression: "'USD'"
          - id: CUSTOMER_ACTIVE_WINDOW.START
            transformation_expression: created_at
          - id: CUSTOMER_ACTIVE_WINDOW.END
            transformation_expression: deactivated_at
            attribute_effective_timestamp_expression: deactivated_at
        relationships:
          - id: CUSTOMER_PLACES_ORDER
            target_transformation_expression: "'ORDER:' || CAST(latest_order_id AS VARCHAR)"

  - name: stripe
    allow_multiple_identifiers: false
    tables:
      - source: stripe
        entity: customer
        from: current
        primary_keys:
          # Stripe carries the AdventureWorks customer_id as metadata.aw_customer_id;
          # using the same canonical-IDFR formula collapses both rows onto the same surrogate.
          - "'CUSTOMER:' || aw_customer_id"
        entity_effective_timestamp_expression: updated
        attributes:
          - id: CUSTOMER_NAME
            transformation_expression: name
          - id: CUSTOMER_LIFETIME_VALUE.AMOUNT
            transformation_expression: total_revenue / 100.0
          - id: CUSTOMER_LIFETIME_VALUE.CURRENCY
            transformation_expression: currency
```

### Field reference

#### `entity:`

| Field | Required | Purpose |
|---|---|---|
| `id` | yes | UPPER_SNAKE business identifier; drives table/view names (lower-cased to `dab.<entity>__*`) and TYPE_KEY hashing. |
| `name` | yes | Display name. Distinct from `id` in DMDL but typically equal in practice. |
| `definition` | yes | One-line technical definition. |
| `description` | no | Longer prose. |

#### `attributes[]`

Each attribute either declares one inner type directly OR declares a `group:` of inner attributes (the atomic context).

| Field | Required | Purpose |
|---|---|---|
| `id` | yes | UPPER_SNAKE attribute identifier; drives `TYPE_KEY` hashing and `__<group>` view name (lower-cased). |
| `definition` | yes | One-line technical definition. |
| `description` | no | Longer prose. |
| `type` | yes (when no `group:`) | One of `STRING`, `NUMBER`, `UNIT`, `START_TIMESTAMP`, `END_TIMESTAMP`. Single-attribute group. |
| `group[]` | yes (when no `type:`) | Inner attributes. Each has its own `id` (scoped to the outer group) and `type`. Max one of each `type` per group (DMDL constraint). |
| `effective_timestamp` | no | If `true`, the descriptor row carries an EFF_TMSTP and the analyst-facing typed view exposes time-aware semantics. Default `false` only makes sense for unchanging attributes (rare in M2 — keep `true` unless you have a reason). |

#### `relationships[]`

| Field | Required | Purpose |
|---|---|---|
| `id` | yes | UPPER_SNAKE relationship identifier; drives `__<related>__rel` table naming and `TYPE_KEY` hashing for the relationship row. |
| `definition` | yes | One-line technical definition. |
| `target_entity_id` | yes | UPPER_SNAKE `id` of the focal on the other side. Must be a focal declared by another `contracts/dab/<entity>.yml` (or by this same one for self-relations). |

The "source" side of a relationship is implicit — it's the focal of the file you're in. There is no `source_entity_id:` (DMDL has it; we drop it because the surrounding file already declares the source focal).

#### `mapping_groups[]`

A focal can be fed by multiple DAS sources. Each `mapping_groups[]` entry is one logical contribution path.

| Field | Required | Purpose |
|---|---|---|
| `name` | yes | snake_case. Used as `INST_KEY` for rows produced by this group; useful for provenance queries. |
| `allow_multiple_identifiers` | no, default `false` | When `true`, multiple `tables[]` in this group can produce different `<entity>_idfr` strings that all resolve to the same surrogate via `<entity>_idfr` collapsing in `__idfr` rows. Irreversible (DMDL semantics — flipping back retroactively requires manual descriptor rebuild). M2 supports `false` only; `true` is a future toggle. |
| `tables[]` | yes | DAS sources contributing to this mapping group. |

#### `mapping_groups[].tables[]`

| Field | Required | Purpose |
|---|---|---|
| `source` | yes | DAS source ID (directory name in `contracts/das/`). Validated. |
| `entity` | yes | DAS entity ID (file basename in `contracts/das/<source>/`). Validated. |
| `from` | no, default `current` | `current` reads `das__<source>.<entity>__current` (one row per PK). `historized` reads `__historized` (full audit log; populates DAB with one descriptor row per typed observation, preserving history). |
| `primary_keys[]` | yes | One or more SQL expressions over DAS columns that jointly produce the canonical IDFR string for this contribution. List length ≥1. Single-element list: the expression value IS the IDFR. Multi-element list: prism concatenates the cast-to-VARCHAR results with the separator `\|\|__\|\|` (literal four-char string `||__||`, chosen to avoid collisions with snake_case content and SQL `\|\|` operator characters appearing in raw data). Users who prefer to control the separator themselves can express composites in a single element (e.g. `order_id \|\| '-' \|\| order_item_id`); the list-with-separator form exists for legibility. The resulting string is the `<entity>_idfr` value; `MD5(<entity>_idfr)` is the surrogate `<entity>_key`. |
| `entity_effective_timestamp_expression` | no | SQL expression over DAS columns producing a `TIMESTAMP`. Default EFF_TMSTP for every descriptor in this table unless the attribute overrides. Falls back to DAS `_loaded_at` when omitted. |
| `where` | no | SQL filter over DAS columns. Rows for which this evaluates to `FALSE` or `NULL` are skipped. |
| `attributes[]` | yes (≥1) | Bindings from declared model attributes to DAS columns. |
| `relationships[]` | no | Bindings from declared model relationships to DAS columns producing the related focal's IDFR string. |

#### `mapping_groups[].tables[].attributes[]`

Each entry binds one model attribute (or one inner group member) to a SQL expression over the DAS table.

| Field | Required | Purpose |
|---|---|---|
| `id` | yes | Either the model attribute's `id` (for single-type attributes) or `<attribute_id>.<inner_id>` (for group members). |
| `transformation_expression` | yes | SQL expression evaluated against the DAS row. Multiline allowed via `>` or `\|` YAML scalar styles. |
| `where` | no | Per-attribute filter. Skips this attribute's descriptor row when false; does NOT skip the whole entity. |
| `attribute_effective_timestamp_expression` | no | Overrides `entity_effective_timestamp_expression` for this attribute's group. |

For an atomic-context group, **every inner member must be mapped** in a single contribution (the group lands as one descriptor row; partial groups are a validation error). The group's `EFF_TMSTP` is single-valued. Resolution rule: if any inner member specifies `attribute_effective_timestamp_expression`, **all members that specify it must specify the same expression** (string-equal); if none specify it, the table-level `entity_effective_timestamp_expression` (or DAS `_loaded_at` fallback) is used. Mismatches are a validate-time error, not silently resolved.

#### `mapping_groups[].tables[].relationships[]`

| Field | Required | Purpose |
|---|---|---|
| `id` | yes | Model relationship's `id`. |
| `target_transformation_expression` | yes | SQL expression producing the **target focal's canonical IDFR string** — the same string the target focal's mapping uses to derive its surrogate. Hashed (MD5) the same way to land the target's `<target_entity>_key` value. |
| `where` | no | Skip the relationship row when false. |

## CLI surface (M2 additions)

| Command | Purpose |
|---|---|
| `prism dab discover` | For each DAS source/entity that doesn't already appear in `contracts/dab/`, propose a focal scaffold at `contracts/dab/<entity>.yml`. Generates an `entity:` block, a flat `attributes[]` list (one STRING/NUMBER/TIMESTAMP attribute per DAS column, types inferred from DAS), and one `mapping_groups[]` entry binding each model attribute to its DAS column. **Does not** propose cross-source unification, atomic-context groups, or relationships — those require business judgment. Skips files that already exist; `--update` does drift detection (added/removed DAS columns since last discover) and surfaces them as `# TODO:` comments inline. |
| `prism dab build [<entity>] [--all]` | Generate SQL from contracts, apply to DuckDB → IDFR + Focal + Descriptor + Relationship tables and per-group typed views and the per-entity `__current` view. Idempotent. |
| `prism dab run [<entity>] [--all]` | Alias for `prism dab build`. M2 has no land step (DAB reads from DAS, which already landed). The alias exists for surface symmetry with `prism das run`. |
| `prism run` | Now equivalent to `prism das run --all && prism dab run --all`. |

`prism validate` extends to cover DAB contracts. `prism doctor` extends to cover cross-layer references (every `mapping_groups[].tables[]` `source`/`entity` resolves to a known DAS contract).

## DuckDB output

For each focal entity, `prism dab build` applies one DDL operation per object (idempotent via `IF NOT EXISTS` / `CREATE OR REPLACE`) followed by content merges. All SQL is templated and emitted via the Engine interface.

### 1. Ensure schema

```sql
CREATE SCHEMA IF NOT EXISTS dab;
```

Single, source-agnostic. Different from M1, where each source got its own `das__<source>` schema.

### 2. IDFR table — one row per (source-derived canonical IDFR, mapping group)

```sql
CREATE TABLE IF NOT EXISTS dab.{entity}__idfr (
    {entity}_key       VARCHAR  NOT NULL,         -- MD5({entity}_idfr)
    {entity}_idfr      VARCHAR  NOT NULL,
    eff_tmstp          TIMESTAMP NOT NULL,
    ver_tmstp          TIMESTAMP NOT NULL,
    seq_nbr            BIGINT    NOT NULL,
    row_st             CHAR(1)   NOT NULL,        -- 'Y' active, 'N' superseded
    data_key           VARCHAR   NOT NULL,        -- 'dab'
    inst_key           VARCHAR   NOT NULL,        -- mapping_group name
    inst_row_key       VARCHAR   NOT NULL,        -- '<source>.<entity>' the row came from
    popln_tmstp        TIMESTAMP NOT NULL,        -- build run wall clock
    PRIMARY KEY ({entity}_key, {entity}_idfr, ver_tmstp)
);
```

The IDFR table holds **every business identifier ever observed** for the focal across every source. Multiple rows with the same `{entity}_key` and different `{entity}_idfr` are valid and indicate aliases (e.g. one row from AdventureWorks, one from Stripe, sharing the same canonical IDFR formula → same surrogate). `ROW_ST = 'Y'` on the latest version of each `({entity}_key, {entity}_idfr)`; older versions get `'N'` on the next build.

### 3. Focal table — one row per surrogate key

```sql
CREATE TABLE IF NOT EXISTS dab.{entity} (
    {entity}_key       VARCHAR  NOT NULL PRIMARY KEY,
    eff_tmstp          TIMESTAMP NOT NULL,        -- earliest IDFR eff_tmstp for this key
    ver_tmstp          TIMESTAMP NOT NULL,
    row_st             CHAR(1)   NOT NULL,
    data_key           VARCHAR   NOT NULL,
    popln_tmstp        TIMESTAMP NOT NULL
);
```

Sparse by design: focal carries identity only. Descriptor data lives in the descriptor table; analyst-friendly access is via the `__current` view.

### 4. Descriptor table — generic EAV, one row per atomic-context observation

```sql
CREATE TABLE IF NOT EXISTS dab.{entity}__descriptor (
    {entity}_key       VARCHAR   NOT NULL,
    type_key           VARCHAR   NOT NULL,        -- MD5('<entity_id>:<attribute_id>')
    eff_tmstp          TIMESTAMP NOT NULL,
    ver_tmstp          TIMESTAMP NOT NULL,
    seq_nbr            BIGINT    NOT NULL,
    row_st             CHAR(1)   NOT NULL,
    sta_tmstp          TIMESTAMP,                 -- group's START_TIMESTAMP attribute, if any
    end_tmstp          TIMESTAMP,                 -- group's END_TIMESTAMP attribute, if any
    val_str            VARCHAR,                   -- group's STRING attribute, if any
    val_num            DOUBLE,                    -- group's NUMBER attribute, if any
    uom                VARCHAR,                   -- group's UNIT attribute, if any
    data_key           VARCHAR   NOT NULL,
    inst_key           VARCHAR   NOT NULL,
    inst_row_key       VARCHAR   NOT NULL,
    popln_tmstp        TIMESTAMP NOT NULL,
    PRIMARY KEY ({entity}_key, type_key, eff_tmstp, ver_tmstp)
);
```

One physical row per (entity, atomic-context group, effective time, version). Per-attribute slots (`val_str`, `val_num`, `uom`, `sta_tmstp`, `end_tmstp`) are populated according to the attribute types declared in the group; unused slots are `NULL`. The DMDL constraint "max one of each type per group" guarantees this fits in five generic slots.

### 5. Relationship table — one per declared relationship

```sql
CREATE TABLE IF NOT EXISTS dab.{entity}__{related}__rel (
    {entity}_key         VARCHAR   NOT NULL,
    {related}_key        VARCHAR   NOT NULL,
    type_key             VARCHAR   NOT NULL,      -- MD5('<entity_id>:<relationship_id>')
    eff_tmstp            TIMESTAMP NOT NULL,
    ver_tmstp            TIMESTAMP NOT NULL,
    seq_nbr              BIGINT    NOT NULL,
    row_st               CHAR(1)   NOT NULL,
    data_key             VARCHAR   NOT NULL,
    inst_key             VARCHAR   NOT NULL,
    inst_row_key         VARCHAR   NOT NULL,
    popln_tmstp          TIMESTAMP NOT NULL,
    PRIMARY KEY ({entity}_key, {related}_key, type_key, eff_tmstp, ver_tmstp)
);
```

Multiple relationships from the same focal to different targets each get their own table. The relationship `id` is encoded only in `type_key`, but the table name uses the target focal's `id` for analyst readability — when one focal has multiple relationships to the same target, the relationship `id` (lower-cased) is appended: `dab.{entity}__{related}__{relationship_id}__rel`. `SEQ_NBR` and `ROW_ST` here partition by `(<entity>_key, <related>_key, type_key)` — the same edge observed at multiple effective times produces a sequence; only the latest is `'Y'`.

### 6. Per-group typed views — analyst surface

For each atomic-context group on each focal:

```sql
CREATE OR REPLACE VIEW dab.{entity}__{attribute_id} AS
SELECT
    {entity}_key,
    eff_tmstp,
    ver_tmstp,
    row_st,
    -- typed projections per group member, NULL where the type slot is unused:
    val_str  AS {string_member_id},     -- when group has a STRING member
    val_num  AS {number_member_id},     -- when group has a NUMBER member
    uom      AS {unit_member_id},       -- when group has a UNIT member
    sta_tmstp AS {start_member_id},     -- when group has a START_TIMESTAMP member
    end_tmstp AS {end_member_id}        -- when group has an END_TIMESTAMP member
FROM dab.{entity}__descriptor
WHERE type_key = {hash('<entity_id>:<attribute_id>')};
```

Single-type groups expose a single typed column named after the attribute (verbatim, lower-cased). Atomic groups expose one column per inner member, named by the inner `id` (verbatim, lower-cased).

**Naming rule (typed views):** `dab.<lower_entity_id>__<lower_attribute_id>`. No prefix stripping — if the user names an attribute `CUSTOMER_NAME` on entity `CUSTOMER`, the view is `dab.customer__customer_name`. Users who want shorter names can declare attribute IDs without the entity prefix (`id: NAME` → `dab.customer__name`). The rule is verbatim because verbatim is unambiguous; ergonomics is the user's choice.

### 7. Per-focal current view — entity-shaped, ROW_ST='Y' only

```sql
CREATE OR REPLACE VIEW dab.{entity}__current AS
SELECT
    f.{entity}_key,
    -- one column per attribute (single-type) or per group member (atomic group);
    -- column-naming rule below; NULL when no active descriptor:
    n.customer_name,
    lv.amount    AS customer_lifetime_value__amount,
    lv.currency  AS customer_lifetime_value__currency,
    aw.start     AS customer_active_window__start,
    aw."end"     AS customer_active_window__end
FROM dab.{entity} f
LEFT JOIN dab.customer__customer_name           n  ON n.{entity}_key  = f.{entity}_key  AND n.row_st  = 'Y'
LEFT JOIN dab.customer__customer_lifetime_value lv ON lv.{entity}_key = f.{entity}_key AND lv.row_st = 'Y'
LEFT JOIN dab.customer__customer_active_window  aw ON aw.{entity}_key = f.{entity}_key AND aw.row_st = 'Y'
WHERE f.row_st = 'Y';
```

**Naming rule (`__current` columns):** `<lower_attribute_id>` for single-type attributes, `<lower_attribute_id>__<lower_inner_id>` for atomic-group members (double-underscore separator, matching ADR-005). The view is a convenience for the common "give me the current state" query; analysts who need bi-temporal queries hit `__descriptor` or the per-group views directly.

## Bi-temporal columns

All four bi-temporal columns are **mechanically derived** from the contract and DAS metadata. Users do not write them in mappings.

| Column | Source | Notes |
|---|---|---|
| `EFF_TMSTP` | `attribute_effective_timestamp_expression` (per attribute) → `entity_effective_timestamp_expression` (per table) → DAS `_loaded_at` (fallback) | "When did this fact become true in the business sense?" Cast to `TIMESTAMP`. |
| `VER_TMSTP` | Build-run wall clock (one constant per `prism dab build` invocation) | "When did our system observe this version?" Identical for all rows produced by the same build. |
| `SEQ_NBR` | `ROW_NUMBER() OVER (PARTITION BY <entity>_key, type_key ORDER BY EFF_TMSTP, VER_TMSTP)` | Used for delta-detection hashing and stable ordering. |
| `ROW_ST` | `'Y'` for the latest per `(<entity>_key, type_key)` ordered by `EFF_TMSTP DESC, VER_TMSTP DESC`; `'N'` otherwise | Recomputed on every build over the full descriptor table; previously-active rows transition to `'N'` when superseded. |

`STA_TMSTP` and `END_TMSTP` (start/end of validity) live on the descriptor row itself when the atomic-context group declares `START_TIMESTAMP` / `END_TIMESTAMP` members. They are populated from the corresponding mapped attribute's `transformation_expression`.

The audit columns (`DATA_KEY`, `INST_KEY`, `INST_ROW_KEY`, `POPLN_TMSTP`) are also derived: `DATA_KEY = 'dab'`, `INST_KEY = <mapping_group.name>`, `INST_ROW_KEY = '<source>.<entity>'`, `POPLN_TMSTP = build wall clock`.

### Build idempotency

A build is a closed-form computation: given DAS staging tables and contracts, the resulting DAB tables are uniquely determined. Two consecutive builds with no DAS change produce identical content (apart from `VER_TMSTP` / `POPLN_TMSTP`, which are stamped per build but don't affect downstream `ROW_ST` selection because all rows from the same build share them).

To keep DAB stable across no-op builds, the merge logic skips rewriting rows whose **content hash** matches an existing row's content hash. Content hash for descriptors is `MD5(type_key || coalesce(val_str,'') || coalesce(val_num::VARCHAR,'') || coalesce(uom,'') || coalesce(sta_tmstp::VARCHAR,'') || coalesce(end_tmstp::VARCHAR,'') || eff_tmstp::VARCHAR)`. If that hash already exists for the `(<entity>_key, type_key)`, the build short-circuits insert.

## Internals

### Engine interface extensions

Additive to M1. M1 methods unchanged.

```go
// internal/engine/engine.go (M2 additions)

type Dialect interface {
    // ... M1 methods ...

    CreateIdfrTableIfNotExists(spec IdfrTableSpec) string
    CreateFocalTableIfNotExists(spec FocalTableSpec) string
    CreateDescriptorTableIfNotExists(spec DescriptorTableSpec) string
    CreateRelationshipTableIfNotExists(spec RelationshipTableSpec) string

    MergeIdfr(spec MergeIdfrSpec) string
    MergeFocal(spec MergeFocalSpec) string
    MergeDescriptor(spec MergeDescriptorSpec) string
    MergeRelationship(spec MergeRelationshipSpec) string

    CreateOrReplaceGroupView(spec GroupViewSpec) string
    CreateOrReplaceEntityCurrentView(spec EntityCurrentViewSpec) string

    HashFunction(input string) string   // dialect-specific MD5 invocation
}

type IdfrTableSpec struct {
    Schema    string             // "dab"
    Entity    string             // "customer" (lower-snake)
}

type FocalTableSpec struct {
    Schema string
    Entity string
}

type DescriptorTableSpec struct {
    Schema string
    Entity string
}

type RelationshipTableSpec struct {
    Schema       string
    Entity       string             // source-side focal
    Related      string             // target-side focal (lower-snake)
    Suffix       string             // optional, when same-target relationship id disambiguator needed
}

type MergeIdfrSpec struct {
    Schema       string
    Entity       string
    SourceQuery  string             // SELECT producing (idfr, eff_tmstp, inst_row_key, ...)
    MappingGroup string             // INST_KEY
}

type MergeDescriptorSpec struct {
    Schema       string
    Entity       string
    SourceQuery  string             // SELECT producing one row per group observation, with type_key already hashed
    MappingGroup string
}

// (analogous for MergeFocal, MergeRelationship)

type GroupViewSpec struct {
    Schema       string
    Entity       string
    AttributeID  string             // upper-snake; lower-cased in view name
    TypeKeyHash  string             // MD5('<entity_id>:<attribute_id>')
    Members      []GroupMember      // one per inner attribute (or one for single-type)
}

type GroupMember struct {
    InnerID string                  // upper-snake (single-type: same as outer ID)
    Type    string                  // STRING|NUMBER|UNIT|START_TIMESTAMP|END_TIMESTAMP
}

type EntityCurrentViewSpec struct {
    Schema       string
    Entity       string
    Groups       []GroupMember      // outer attributes (with members embedded)
}
```

DuckDB implementation lives in `internal/engine/duckdb/duckdb.go`. SQL templates live under `internal/tmpl/duckdb/dab/*.sql.tmpl`.

### Mapping execution

For each entity contract, `prism dab build` does (per mapping group, per table):

1. **Build a single CTE-per-table** that selects from `das__<source>.<entity>__current` (or `__historized`), applies the table-level `where`, and projects:
   - `<entity>_idfr := <primary_keys[0] || '__//__' || ...>`
   - `<entity>_key := MD5(<entity>_idfr)`
   - `entity_eff_tmstp := <entity_effective_timestamp_expression OR _loaded_at>`
   - One column per mapped attribute (with per-attribute `where` and `attribute_effective_timestamp_expression`).
2. **Emit the IDFR merge** by selecting `(<entity>_key, <entity>_idfr, entity_eff_tmstp)` from the CTE.
3. **Emit the focal merge** by selecting distinct `(<entity>_key, MIN(entity_eff_tmstp))` from the CTE.
4. **For each declared atomic-context group**, emit one descriptor merge selecting `(<entity>_key, type_key=<hash>, eff_tmstp, val_str, val_num, uom, sta_tmstp, end_tmstp)` from the CTE. If any of the group's inner-member `where` clauses is false for a row, the descriptor row for that group is skipped (the entity itself is still produced).
5. **For each declared relationship**, emit one relationship merge selecting `(<entity>_key, target_key=MD5(target_transformation_expression), type_key=<hash>, eff_tmstp)` from the CTE.
6. **After all mapping groups complete**, recompute `ROW_ST` and `SEQ_NBR` over the full descriptor / IDFR / relationship tables for the affected `(<entity>_key, type_key)` cohorts via `UPDATE … FROM (SELECT … ROW_NUMBER() OVER …)` patterns. Single-statement, idempotent.
7. **Render or refresh** the per-group typed views and the per-entity `__current` view.

All steps run inside a single DuckDB transaction per entity. A failure in one entity does not corrupt others.

### TYPE_KEY hashing

`type_key` values are deterministic strings, computed by prism at build time:

- For a descriptor type: `MD5('<entity_id>:<attribute_id>')` — e.g. `MD5('CUSTOMER:CUSTOMER_LIFETIME_VALUE')`.
- For a relationship type: `MD5('<entity_id>:<relationship_id>')` — e.g. `MD5('CUSTOMER:CUSTOMER_PLACES_ORDER')`.

Because they are deterministic functions of contract content, no `_types.yml` registry is needed. Renaming an attribute changes its `type_key`; downstream `__descriptor` rows that referenced the old hash become orphaned (their `ROW_ST` will not be touched on subsequent builds, since they no longer match any contract-derived type_key). Treat rename as a schema migration; document a manual cleanup pattern.

### Validation (extends `prism validate`)

| Check | What |
|---|---|
| YAML parse | Each `contracts/dab/*.yml` parses as YAML. |
| Schema match | Each parses against the embedded JSON Schema (`dab_entity_v1`). |
| Entity ID uniqueness | No two files declare the same `entity.id`. |
| Attribute ID uniqueness | No two attributes within a focal share an `id`; no two inner-group members share an `id` within a group. |
| Group type uniqueness | Within each `group:`, each `type:` appears at most once. |
| Type names known | Every `type:` is `STRING\|NUMBER\|UNIT\|START_TIMESTAMP\|END_TIMESTAMP`. |
| Mapping completeness | Every model attribute (and every inner group member) is bound by **at least one** mapping group's table; unbound attributes warn (allowed, but flagged). |
| Group atomicity | When a group is bound by a table, **all** members of that group are bound in the same table. Partial group binding is an error. |
| Cross-layer references | Every `mapping_groups[].tables[].source` resolves to a directory under `contracts/das/`; every `entity` resolves to a file under that source. |
| `target_entity_id` | Every relationship's `target_entity_id` is a focal declared by some `contracts/dab/*.yml`. |
| `relationships[]` mapping | Every model relationship is mapped by at least one mapping group's table; unmapped relationships warn (allowed). |
| Reserved column names | `transformation_expression` SQL doesn't textually reference identifiers prefixed with `_dlt_` (those are dlt landing metadata, not business data). Substring-level warning, not a SQL parse — false positives possible (e.g. a string literal containing `_dlt_`); accept the trade-off because we have no SQL parser at validate time. |

CI-friendly. Pre-commit-friendly. Runs in milliseconds.

## State

**Prism keeps no state file in M2** (continuing M1's invariant). All DAB state lives in the DuckDB warehouse; rebuilding from scratch by dropping the `dab` schema and re-running `prism dab build` is supported and idempotent.

Re-running `prism dab build` after editing a contract is safe:
- New attribute → new descriptor merge runs; existing `__descriptor` rows untouched. View regenerated to expose the new column.
- New mapping group → its rows get merged in; previously-merged rows from other groups untouched. Existing IDFR/Focal/Descriptor rows revalidated for `ROW_ST` against the now-larger pool.
- Attribute renamed → old `type_key` rows orphan (no longer addressed by views; not deleted). User drops `dab.<entity>__descriptor` and rebuilds, or accepts orphan rows.
- Attribute retyped (e.g. `STRING` → `NUMBER`) → DDL on the descriptor table is unchanged (it's generic), but the typed view changes shape. `prism dab build` re-renders both the `dab.<entity>__<attribute_id>` and `dab.<entity>__current` views via `CREATE OR REPLACE VIEW`, picking up the new shape on the next run. Old rows where `val_str` carries the now-numeric data become orphaned reads (visible only via raw `__descriptor` access); document.
- Mapping group removed → rows produced by that group linger (their `INST_KEY` no longer matches any contract). M2 does not auto-prune. Deferred to M4 (`prism dab prune`).

## Doctor checks (M2 additions)

`prism doctor` extends with:

- For each `contracts/dab/*.yml`, every `mapping_groups[].tables[]` `source`/`entity` resolves to a known DAS contract. (Validation already does this; doctor verifies the DAS staging table also exists in DuckDB — warns if it doesn't, since that means `prism das build` hasn't been run for that source.)
- For each declared focal, the cross-references between focals (relationships' `target_entity_id`) form an acyclic graph at the model level (relationships are unidirectional; cycles allowed at data level via separate relationships in each direction, but the contract graph is a DAG of declared edges).

## Testing

Three tiers, mirroring M1.

### Tier 1 — Static validation (no DB, no network)

`prism validate` covers all M2 checks listed above plus M1's. Runs in milliseconds.

### Tier 2 — Wiring tests (DB roundtrip, fixture DAS)

Go-level tests inside the prism Go repo:

| Test | What |
|---|---|
| SQL template snapshots | Render every M2 template (CreateIdfrTable, CreateFocalTable, CreateDescriptorTable, CreateRelationshipTable, MergeIdfr, MergeDescriptor, MergeRelationship, CreateOrReplaceGroupView, CreateOrReplaceEntityCurrentView) for representative inputs; assert against committed golden files. |
| Engine round-trip — single source | Open temp DuckDB, populate `das__<source>.customer__current` with a fixture, apply rendered DAB DDL + merge, verify IDFR / Focal / Descriptor rows and per-group views return expected typed projections. |
| Engine round-trip — multi-source unification | Two DAS sources, contracts whose `primary_keys` produce the same canonical IDFR string for matching rows. Assert one focal row per business identity, two IDFR rows (one per source), descriptor rows merged. |
| Engine round-trip — atomic-context group | A focal with `CUSTOMER_LIFETIME_VALUE.AMOUNT` + `CURRENCY` group, mapped from DAS. Assert one descriptor row per observation, with `val_num` + `uom` populated; assert the `dab.customer__customer_lifetime_value` view exposes `amount` + `currency` typed columns. |
| Engine round-trip — bi-temporal | Two builds with DAS changes between them. Assert ROW_ST flips correctly; SEQ_NBR sequences correctly; previously-active rows are now `'N'` and the new row is `'Y'`. |
| Engine round-trip — relationships | A `CUSTOMER_PLACES_ORDER` relationship; assert relationship rows land with both surrogate keys correctly hashed from canonical IDFR strings. |
| Idempotency | Two consecutive `prism dab build` invocations on unchanged DAS state produce identical descriptor/IDFR/relationship content (modulo `VER_TMSTP`/`POPLN_TMSTP`). |
| Validation negative tests | Partial group mapping, unknown type, dangling source/entity reference, missing `target_entity_id` — each fails validation with a clear error message. |
| `prism dab discover` | Against fixture DAS contracts, assert generated `contracts/dab/*.yml` parses, scaffolds one attribute per DAS column, and round-trips through validate. |

### Tier 3 — End-to-end against AdventureWorks

Extends the M1 e2e test:

```
prism init  →
prism das discover adventure_works (M1)  →
prism das run adventure_works (M1)  →
prism dab discover                       (M2 — generates contracts/dab/{customer,product,sales_order_header}.yml scaffolds)  →
hand-author atomic-context groups + relationships in those scaffolds  →
prism validate  →  prism dab run --all  →
assert dab.customer__current returns one row per AW customer with descriptors populated
assert dab.customer__sales_order_header__rel rows match AW's customer-order foreign keys
assert ROW_ST = 'Y' on every latest row, no orphan 'Y'-rows
```

Run on schedule (nightly) and on release tags. Network access only via M1's DAS land step; DAB itself is offline.

### Test fixtures (additions to M1's `testdata/`)

- `testdata/contracts/valid/dab/<scenario>.yml` — pairs of DAS + DAB contracts demonstrating single-source, multi-source unification, atomic groups, and relationships.
- `testdata/contracts/invalid/dab/<scenario>.yml` — one rule violation each.
- A handful of in-memory DAS fixture rows for engine round-trip tests (no JSONL needed; we populate `das__<source>.<entity>__current` directly via INSERT in test setup).

## What M2 deliberately does NOT include

- **Curated alias / xref contracts** for the messy "two systems, no shared field" entity-resolution case. M2 supports cross-source unification only when the user can derive the same canonical IDFR string from each source via SQL expressions in `primary_keys`. When that's not possible, the user has two paths (both deferred): (a) carry an alias xref column in DAS itself; (b) wait for `contracts/dab/_aliases/<entity>.yml` in M3+.
- **`allow_multiple_identifiers: true`** — the toggle is reserved in the schema but rejected at validate-time in M2. (The deterministic-hash-of-canonical-IDFR design produces one IDFR row per source contribution naturally; the multi-identifier toggle's value emerges only with the alias-contract feature, which is deferred.)
- **DAB→DAB derived focals** — focals computed from other focals (e.g. an `ACCOUNT` focal derived as the union of `CUSTOMER` and `VENDOR`). Mappings read from DAS only.
- **Schema-evolution migrations** — renaming an attribute, retyping a group member, or removing a mapping group requires manual `dab.*` cleanup (at least: `DROP TABLE dab.<entity>__descriptor` + rebuild). Automated migration deferred to M4.
- **Custom `latest_by:` ordering for `ROW_ST`** — always ordered by `EFF_TMSTP DESC, VER_TMSTP DESC`. User-declared tiebreakers post-M2.
- **Auto-pruning of orphan rows** — when a contract is removed or an attribute renamed, descriptor rows with the old `type_key` linger. `prism dab prune` deferred to M4.
- **Bi-temporal queries from the CLI** — `prism` does not surface "what did the warehouse look like at time T?" tooling. Analysts query the descriptor table directly with `EFF_TMSTP` / `VER_TMSTP` predicates.
- **Cross-focal cardinality validation** — relationships are not checked for "every CUSTOMER_PLACES_ORDER row has a matching ORDER focal" at build time. Failed lookups produce relationship rows with a `<related>_key` that doesn't appear in `dab.<related>` — surfaces as a query-time issue, optionally a tier-1 contract test post-M2.

## Known caveats

- **Surrogate key portability** — `MD5(canonical_idfr)` produces a 32-char hex string. Stable across DuckDB versions, stable across prism versions (the hash function is locked in the dialect). Changing the IDFR formula in a contract changes every surrogate key for that focal; treat as a schema migration.
- **Atomic-context EAV trade-off** — descriptor table accommodates ≤5 typed slots (`val_str`, `val_num`, `uom`, `sta_tmstp`, `end_tmstp`). A future group with two STRING members or a NUMBER + STRING + START_TIMESTAMP + END_TIMESTAMP + UNIT combo is fine. A group requiring two NUMBER members is not — DMDL's "max one of each type per group" constraint is what makes this five-slot table sufficient. Document.
- **Relationship table cardinality** — one relationship row per (entity_key, related_key, type_key, eff_tmstp, ver_tmstp). Many-to-many relationships across a long history can grow large; expected, not optimized in M2.
- **`from: historized` performance** — reads every audit-log row from DAS into the descriptor merge. For high-volume sources, this can be slow. M2 documents the trade-off; performance optimization (incremental builds keyed on DAS `_loaded_at`) deferred to M4.
- **No multi-warehouse concurrency** — same DuckDB single-file lock applies.

## Seams for M3

DAR will plug in by:

1. Adding a layer namespace — `contracts/dar/` alongside `contracts/dab/`.
2. Reading from `dab.*` views (per-group typed views and per-focal `__current` views are the natural surface; raw `__descriptor` for full bi-temporal queries).
3. Producing the unified `dar` schema: a Puppini bridge fact joining all focals, conformed dimensions per business concept, declarative measures.
4. Extending the `Engine` and `Dialect` interfaces with DAR-specific operations (likely `CreateBridgeTable`, `CreateDimension`, `CreateFactView`).
5. Adding a layer to `prism run` — `das run --all → dab run --all → dar run --all`.

DAB's per-focal `__current` views are intentionally analyst-shaped so DAR can build dimensions on top of them with minimal SQL gymnastics. The EAV `__descriptor` table is available when DAR needs SCD-2 history.

Nothing in M2 forecloses M3.
