import io
import json

from prism_dlt_runner import events


def test_emit_writes_jsonl(tmp_path):
    buf = io.StringIO()
    em = events.Emitter(stream=buf)
    em.source_start("adventure_works")
    em.entity_start("Customer")
    em.entity_progress("Customer", rows=100)
    em.entity_end("Customer", rows=123, load_id="L1", files=2)
    em.source_end("adventure_works", entities=1, duration_ms=42)
    em.error("Customer", kind="http_404", message="not found")

    lines = [json.loads(line) for line in buf.getvalue().splitlines()]
    assert lines[0] == {"event": "source.start", "source": "adventure_works"}
    assert lines[1] == {"event": "entity.start", "entity": "Customer"}
    assert lines[2] == {"event": "entity.progress", "entity": "Customer", "rows": 100}
    assert lines[3] == {
        "event": "entity.end", "entity": "Customer", "rows": 123, "load_id": "L1", "files": 2
    }
    assert lines[4] == {
        "event": "source.end", "source": "adventure_works", "entities": 1, "duration_ms": 42
    }
    assert lines[5] == {
        "event": "error", "entity": "Customer", "kind": "http_404", "message": "not found"
    }
