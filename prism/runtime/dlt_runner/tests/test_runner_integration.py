"""Non-mocked integration test: verify that pipeline.run() accepts the kwargs
prism uses and produces JSONL output files.

This is the test that would have caught C1 (invalid kwargs to pipeline.run()).
"""

from __future__ import annotations

import pytest

dlt = pytest.importorskip("dlt", reason="dlt not installed")


def test_pipeline_run_with_prism_kwargs(tmp_path):
    """Real dlt pipeline run with prism's write_disposition + loader_file_format."""

    @dlt.resource(name="items")
    def _items():
        yield {"id": 1, "value": "alpha"}
        yield {"id": 2, "value": "beta"}

    pipeline = dlt.pipeline(
        pipeline_name="prism_integration_test",
        destination=dlt.destinations.filesystem(bucket_url=str(tmp_path)),
        dataset_name="test_source",
    )

    # These are the only kwargs prism now passes to pipeline.run(); verify no TypeError.
    info = pipeline.run(
        _items(),
        write_disposition="append",
        loader_file_format="jsonl",
    )

    assert info is not None

    # At least one .jsonl file should exist under tmp_path.
    jsonl_files = list(tmp_path.rglob("*.jsonl"))
    assert len(jsonl_files) >= 1, f"No .jsonl files found under {tmp_path}"
