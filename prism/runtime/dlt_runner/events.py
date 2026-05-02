"""Structured stdout event emitter for the prism dlt runner.

Events are JSON objects, one per line, written to a configurable stream
(stdout in production, StringIO in tests). See ADR-006 and the design spec
section on IPC for the full event vocabulary.
"""

from __future__ import annotations

import json
import sys
from typing import IO, Any


class Emitter:
    def __init__(self, stream: IO[str] | None = None) -> None:
        self._stream = stream if stream is not None else sys.stdout

    def _emit(self, payload: dict[str, Any]) -> None:
        self._stream.write(json.dumps(payload, separators=(",", ":")) + "\n")
        self._stream.flush()

    def source_start(self, source: str) -> None:
        self._emit({"event": "source.start", "source": source})

    def source_end(self, source: str, entities: int, duration_ms: int) -> None:
        self._emit({
            "event": "source.end", "source": source,
            "entities": entities, "duration_ms": duration_ms,
        })

    def entity_start(self, entity: str) -> None:
        self._emit({"event": "entity.start", "entity": entity})

    def entity_progress(self, entity: str, rows: int) -> None:
        self._emit({"event": "entity.progress", "entity": entity, "rows": rows})

    def entity_end(self, entity: str, rows: int, load_id: str, files: int) -> None:
        self._emit({
            "event": "entity.end", "entity": entity,
            "rows": rows, "load_id": load_id, "files": files,
        })

    def error(self, entity: str, kind: str, message: str) -> None:
        self._emit({
            "event": "error", "entity": entity,
            "kind": kind, "message": message,
        })
