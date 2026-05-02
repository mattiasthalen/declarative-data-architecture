"""Verify the OData provider builds a dlt source with prism's invariants."""

from unittest.mock import MagicMock, patch

from prism_dlt_runner.providers import odata


def test_build_source_passes_base_url():
    src_cfg = {"provider": "odata", "base_url": "https://api.example/odata/v1/"}
    entities = [{"name": "Customer"}, {"name": "Product"}]
    with patch.object(odata, "_dlt_rest_api_source") as factory:
        factory.return_value = MagicMock(name="dlt.source")
        odata.build_source(src_cfg, entities)
        factory.assert_called_once()
        kwargs = factory.call_args.kwargs
        assert kwargs["base_url"] == "https://api.example/odata/v1/"
        # Two entity resources requested:
        assert {r["name"] for r in kwargs["resources"]} == {"Customer", "Product"}


def test_invariants_are_returned():
    invariants = odata.PRISM_INVARIANTS
    assert invariants["max_table_nesting"] == 0
    assert invariants["naming_convention"] == "direct"
    assert invariants["loader_file_format"] == "jsonl"
    assert invariants["add_dlt_id"] is True
    assert invariants["add_dlt_load_id"] is True
