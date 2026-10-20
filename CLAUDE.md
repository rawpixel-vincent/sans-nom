@AGENTS.md

<!-- BEGIN shared-agent-memory: managed block, change it upstream; a local edit blocks the next upgrade -->

## Memory

These rules override Claude Code's auto-memory instructions.

- Repository knowledge goes to the shared store [.agents/memory/](.agents/memory/), in the [unit format](.agents/README.md): recipes, traps, decisions, conventions, code standards and project state. Search it first and update the unit that covers the topic.
- Auto-memory (`~/.claude/projects/…/memory/`) keeps only the user's environment setup, personal information and pointers to their own credentials. Move anything else you find there to the shared store.

<!-- END shared-agent-memory -->
