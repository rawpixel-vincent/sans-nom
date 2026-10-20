# sans-nom

<!-- BEGIN shared-agent-memory: managed block, change it upstream; a local edit blocks the next upgrade -->

## Shared memory

- Durable repo knowledge lives in `.agents/memory/` as units: anchors (grep regex + path glob) bind facts the code cannot state to the lines they concern ([unit format](.agents/README.md)). Code wins over prose.
- Before editing a file, `bun run memory:find --at <path>` lists the units bound to it; `bun run memory:read <unit>` prints one with its anchors resolved and the commits since it was verified. Search the store with `rg` like code.
- Write durable repo knowledge (recipes, traps, decisions, conventions, state) there, never to a harness's private memory: only facts the code cannot state, with anchors. Keep it agnostic (no personal names or home paths) and free of secrets.
- Follow-up work you notice but do not do goes in a self-contained `.agents/HANDOFF-<slug>.md`, deleted when done.

<!-- END shared-agent-memory -->
