"""CLI entry: `python -m prism_dlt_runner --source <yaml> --entity <yaml> ... --lake <dir>`."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from . import runner


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser("prism_dlt_runner")
    p.add_argument("--source", required=True, type=Path,
                   help="path to _source.yml")
    p.add_argument("--entity", action="append", default=[], type=Path,
                   help="path to one entity contract (repeatable)")
    p.add_argument("--lake", required=True, type=Path,
                   help="root lake directory")
    args = p.parse_args(argv)
    if not args.entity:
        print('{"event":"error","entity":"(source)","kind":"NoEntities","message":"at least one --entity required"}', file=sys.stdout, flush=True)
        return 2
    try:
        runner.run(
            source_path=args.source, entity_paths=args.entity, lake_dir=args.lake,
        )
    except Exception:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
