"""OData source factory for the prism dlt runner.

Wraps dlt's REST API helper with OData defaults (paging via @odata.nextLink,
JSON envelope under "value"). Returns a dlt.Source ready to pass to
pipeline.run() with PRISM_INVARIANTS.
"""

from __future__ import annotations

from typing import Any

# Lazy import shim so tests can patch the factory without dlt at import time.
def _dlt_rest_api_source(**kwargs: Any) -> Any:
    from dlt.sources.rest_api import rest_api_source  # type: ignore
    return rest_api_source(**kwargs)


PRISM_INVARIANTS: dict[str, Any] = {
    "write_disposition":  "append",
    "loader_file_format": "jsonl",
}


def build_source(src_cfg: dict, entities: list[dict]):
    """Construct a dlt.Source for the given OData endpoint and entity list.

    src_cfg keys: provider (must be "odata"), base_url
    entities: list of {"name": str, "incremental": {...}?} dicts
    """
    if src_cfg.get("provider") != "odata":
        raise ValueError(f"odata.build_source called with provider={src_cfg.get('provider')!r}")
    base_url = src_cfg["base_url"]

    resources: list[dict] = []
    for ent in entities:
        name = ent["name"]
        resource: dict[str, Any] = {
            "name": name,
            "endpoint": {
                "path": name,
                "data_selector": "value",
                "paginator": {
                    "type": "json_response",
                    "next_url_path": "@odata.nextLink",
                },
            },
        }
        inc = ent.get("incremental")
        if inc:
            resource["endpoint"]["incremental"] = {
                "cursor_path": inc["cursor"],
            }
        resources.append(resource)

    return _dlt_rest_api_source(
        base_url=base_url,
        resources=resources,
        max_table_nesting=0,
    )
