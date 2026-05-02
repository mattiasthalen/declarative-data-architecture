import io
import json
from pathlib import Path
from unittest.mock import MagicMock, patch

from prism_dlt_runner import runner


FIX = Path(__file__).parent / "fixtures"


def test_load_contracts():
    src, ents = runner.load_contracts(FIX / "source.yml", [FIX / "customer.yml"])
    assert src["source"]["provider"] == "odata"
    assert ents[0]["entity"]["name"] == "Customer"
    assert ents[0]["incremental"]["cursor"] == "ModifiedDate"


def test_run_invokes_pipeline_with_invariants(tmp_path):
    buf = io.StringIO()
    fake_pipeline = MagicMock()
    fake_pipeline.run.return_value = MagicMock(loads_ids=["L1"])
    with patch.object(runner, "_make_pipeline", return_value=fake_pipeline) as mp:
        with patch("prism_dlt_runner.providers.odata.build_source") as bs:
            bs.return_value = MagicMock(name="dlt.source")
            runner.run(
                source_path=FIX / "source.yml",
                entity_paths=[FIX / "customer.yml"],
                lake_dir=tmp_path,
                stream=buf,
            )
            bs.assert_called_once()
            mp.assert_called_once()
            fake_pipeline.run.assert_called_once()
            kwargs = fake_pipeline.run.call_args.kwargs
            assert kwargs["loader_file_format"] == "jsonl"
            assert kwargs["write_disposition"] == "append"

    events = [json.loads(l) for l in buf.getvalue().splitlines()]
    kinds = [e["event"] for e in events]
    assert "source.start" in kinds
    assert "source.end" in kinds


def test_unknown_provider_errors():
    src = {"source": {"provider": "mysql", "base_url": "x"}}
    ents = []
    try:
        runner._dispatch_source(src, ents)
    except ValueError as e:
        assert "mysql" in str(e)
    else:
        raise AssertionError("expected ValueError")
