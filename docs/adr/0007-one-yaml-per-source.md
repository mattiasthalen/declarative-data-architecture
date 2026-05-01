# ADR-007: One YAML file per source; source ID derived from filename

**Date:** 2026-05-01
**Status:** Accepted
**Milestone:** M1 (sets convention for all milestones)

## Context

A DAS source declaration carries: provider config (URL, auth, etc.) and a list of entities to land. We need a file layout for `contracts/das/`. Three options:

- **One file per source** — `contracts/das/<source>.yml` containing both provider config and entity list.
- **Two files per source** — `contracts/das/<source>/_source.yml` (provider) + `contracts/das/<source>/entities.yml` (list).
- **One file per entity** — `contracts/das/<source>/<entity>.yml` (Daana-blog style).

The Daana blog's one-file-per-entity pattern makes sense in their world because each contract carries per-column schema (typed columns, source paths, modes, descriptions) — which can grow large. In our world, ADR-003 deferred per-column schema to DAB, so a DAS entity declaration is essentially just a name plus optional incremental cursor. The Daana-style multi-file layout is overkill for that.

The two-file split was initially proposed on the theory that provider config rarely changes while entity lists evolve. The user pushed back: with so little to declare, the split is needless ceremony.

## Decision

**One YAML file per source.** Path: `contracts/das/<source>.yml`. Contains provider config (`source:` block) and entities (`entities:` list).

**Source ID is derived from the filename basename.** `adventure_works.yml` → source ID `adventure_works`. No `id:` field in the YAML itself. Renaming the file renames the source (with corresponding rename of `_lake/das/<source>/` and `_pipelines/<source>/` and the DuckDB schema `das__<source>`).

Validation enforces: filename basename matches `^[a-z][a-z0-9_]*$` (snake_case, leading letter).

## Consequences

**Positive:**

- One file to read for everything about a source.
- One source of truth for the source's identity — the filename. No risk of `id:` field drifting from filename.
- Renaming a source is a single `git mv` (plus prism cleanup of the old DuckDB schema and `_lake` directory if desired).
- Layout is consistent across layers — `contracts/<layer>/<thing>.yml`.
- Adding an entity to a source is a single-line append to `entities:`. Removing one is a single-line delete.

**Negative:**

- Files can grow long for sources with many entities. AdventureWorks has ~70 entity sets; the file is still manageable (one line per entity in the simple case). If a future provider needs significantly more per-entity config, the file gets noisier.
- Less granular git history per entity — adding/removing entities all show up as edits to the same file rather than file additions/deletions.

**Acceptable because:** with no per-column schema in DAS, per-entity declarations are trivially small. If a source ever grows so large or so complex per-entity that this breaks down, we revisit (e.g., split into `contracts/das/<source>/main.yml` + a directory of entity files for that one source). M1 doesn't need that complexity.

## Alternatives considered

**A. Two files per source** (`_source.yml` + `entities.yml`). Marginal benefit at best; entity lists aren't long enough to justify a separate file. Rejected.

**B. One file per entity** (`contracts/das/<source>/<entity>.yml`). Daana-blog pattern. Justified there because of per-column schema; not justified here. Rejected.

**C. Required `id:` field that must match filename.** Belt-and-braces, but the validation rule "filename matches `id:`" is exactly the redundancy this ADR removes. Rejected.

## Related

- [ADR-003](0003-das-owns-no-materialized-data.md) — the no-per-column-schema decision is what makes this layout feasible.
