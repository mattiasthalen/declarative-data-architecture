# Vendored: superpowers

`commands/`, `agents/`, `skills/`, and `hooks/` in this directory are vendored
from the [superpowers](https://github.com/obra/superpowers) plugin (MIT,
Copyright (c) 2025 Jesse Vincent — see `LICENSE.superpowers`).

- Upstream: https://github.com/obra/superpowers
- Vendored at commit: `e7a2d16476bf042e9add4699c9d018a90f86e4a6` (v5.0.7)

## Local modifications

- `hooks/session-start`: the JSON-format detection also treats
  `CLAUDE_PROJECT_DIR` as a Claude Code signal, so the hook works when
  installed as a project-local hook (no `CLAUDE_PLUGIN_ROOT`).
- `settings.json` registers the `SessionStart` hook directly using
  `${CLAUDE_PROJECT_DIR}/.claude/hooks/run-hook.cmd` instead of
  `${CLAUDE_PLUGIN_ROOT}`.

## Updating

To pull in upstream changes:

    git clone --depth 1 https://github.com/obra/superpowers.git /tmp/superpowers
    rm -rf .claude/{skills,commands,agents,hooks}
    cp -r /tmp/superpowers/{skills,commands,agents,hooks} .claude/

Then reapply the `session-start` patch above and bump the commit pin in this
file.
