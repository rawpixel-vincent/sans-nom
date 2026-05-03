# Mechanics

The WHAT of sans-nom. What the player does, what verbs exist, who the cast is, how memory works at the game-design level.

Designer's working subtree — daily iteration happens here. For implementation details (how the agent loop is wired, how Honcho is called), see [`../architecture/`](../architecture/README.md).

## Contents

- [`core-loop.md`](core-loop.md) — observe → choose/write → narrator resolves → honcho updates
- [`verbs/`](verbs/) — one file per player verb
  - [`read.md`](verbs/read.md), [`choose.md`](verbs/choose.md), [`speak-act.md`](verbs/speak-act.md), [`ask-tonto.md`](verbs/ask-tonto.md), [`reflect.md`](verbs/reflect.md), [`recall.md`](verbs/recall.md), [`wait.md`](verbs/wait.md)
- [`input/`](input/) — free-text vs. choices, interpretation prompt, edge cases
- [`player-character/`](player-character/) — Sans-Nom's seed traits, card evolution, multiple-Sans-Noms / save model
- [`cast/`](cast/README.md) — one file per cast member, plus dead/absent + generated NPCs
- [`memory-model/`](memory-model/) — Honcho mapping, sessions, card updates, eviction
- [`world/`](world/) — Bushitaka and IP commitments
