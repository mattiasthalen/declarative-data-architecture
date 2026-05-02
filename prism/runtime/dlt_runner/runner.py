"""Entry point logic for the prism dlt runner.

Reads one `_source.yml` and N `<entity>.yml` files; constructs a dlt.Source
via the appropriate provider factory; runs a dlt.pipeline that writes JSONL
to <lake_dir>/<source_id>/<entity>/. All transformation invariants live here
(see ADR-002, ADR-006).
"""

from __future__ import annotations

import time
from pathlib import Path
from typing import IO, Any

import yaml

from . import events
from .providers import odata as odata_provider


PRISM_INVARIANTS: dict[str, Any] = odata_provider.PRISM_INVARIANTS  # shared


def load_contracts(source_path: Path, entity_paths: list[Path]):
    src = yaml.safe_load(Path(source_path).read_text())
    ents = [yaml.safe_load(Path(p).read_text()) for p in entity_paths]
    return src, ents


def _dispatch_source(src: dict, entities: list[dict]):
    provider = src["source"]["provider"]
    if provider == "odata":
        return odata_provider.build_source(src["source"], [e["entity"] | {"incremental": e.get("incremental")} for e in entities])
    raise ValueError(f"unknown provider {provider!r}; supported: odata")


def _make_pipeline(source_id: str, lake_dir: Path):
    import dlt  # local import; the venv has it
    return dlt.pipeline(
        pipeline_name=f"prism_{source_id}",
        destination=dlt.destinations.filesystem(bucket_url=str(lake_dir.resolve())),
        dataset_name=source_id,
    )


def run(
    *,
    source_path: Path,
    entity_paths: list[Path],
    lake_dir: Path,
    stream: IO[str] | None = None,
) -> None:
    em = events.Emitter(stream=stream)
    src, ents = load_contracts(source_path, entity_paths)
    source_id = Path(source_path).parent.name  # contracts/das/<source_id>/_source.yml

    em.source_start(source_id)
    started = time.monotonic()

    try:
        dlt_source = _dispatch_source(src, ents)
        pipeline = _make_pipeline(source_id, Path(lake_dir))
        for ent in ents:
            em.entity_start(ent["entity"]["name"])
        info = pipeline.run(dlt_source, **PRISM_INVARIANTS)
        load_id = info.loads_ids[0] if getattr(info, "loads_ids", None) else "unknown"
        for ent in ents:
            em.entity_end(ent["entity"]["name"], rows=-1, load_id=load_id, files=-1)
    except Exception as exc:  # pragma: no cover — surface the error to Go side
        em.error(entity="(source)", kind=type(exc).__name__, message=str(exc))
        raise

    em.source_end(
        source_id,
        entities=len(ents),
        duration_ms=int((time.monotonic() - started) * 1000),
    )
