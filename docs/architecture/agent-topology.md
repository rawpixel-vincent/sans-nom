# Agent Topology

```
                   ┌───────────────────────────────┐
                   │           PLAYER              │
                   │  (free-text + choice input)   │
                   └───────────────┬───────────────┘
                                   │
                                   ▼
                   ┌───────────────────────────────┐
                   │         NARRATOR AGENT        │
                   │  (Claude Opus, lore-soaked,   │
                   │   passion-trait peer card)    │
                   └────┬─────────┬────────────────┘
                        │         │
            queries     │         │   commits scene
            actors      │         │   to Honcho
                        ▼         ▼
        ┌───────────────────┐   ┌──────────────────────┐
        │   ACTOR AGENTS    │   │      HONCHO          │
        │ (one per char,    │   │  (memory, peer       │
        │  Sonnet-tier,     │   │   cards, scoped      │
        │  scoped repr)     │   │   representations)   │
        └─────────┬─────────┘   └──────────┬───────────┘
                  │                        │
                  └────────────────────────┘
                  actors read their own
                  representations from Honcho
```

## What each layer does

**The Narrator** is the only agent the player ever sees. Claude Opus tier. Its peer card encodes a *creative-writer-with-passion* persona — specifically, a Metabarons devotee who treats the IP's sacred laws as creative non-negotiables. The Narrator:

- Receives the player's input.
- Decides which actors are present in the current scene.
- Queries those actors privately, providing them with their scoped Honcho representation of the player and any other present characters.
- Synthesizes the actors' outputs into a scene the way a director synthesizes performances.
- Writes the scene's prose. The actors' raw text *never* appears to the player.
- Decides on the choices to offer (2–4 options + free-text).
- Commits the canonical version of the scene to Honcho with the right peer-attribution and witnessing structure.

See [`narrator-agent/`](narrator-agent/) for the Narrator's peer card, authoring loop, failure-mode defenses, and dev-loop ergonomics.

**Actor agents** are one per named character. Sonnet-tier (cost-controlled). Their peer cards are seeded from the user's existing character lore. New characters the Narrator invents during play get peer cards generated on first appearance and persisted thereafter. Each actor:

- Receives only the Narrator's scene-setup prompt and its own Honcho-scoped representation of the player and other present characters.
- Outputs how it would speak/act given that input.
- Never writes prose for the player to read. Its output is raw — interior monologue, dialogue, reaction. The Narrator decides what surfaces.

See [`actor-agents/`](actor-agents/) for invocation contracts and card seeding.

**Honcho** is the memory substrate. Specifically:

- One peer per character (including the player as `sans-nom`).
- `observe_others=true` enabled at the session level for every character present in a scene. This is what produces directional representations.
- Each scene is a message; sessions group scenes by location/arc.
- Peer cards evolve during play via Honcho's *dreaming* + targeted manual updates.
- Directional peer cards (`alice.set_card([...], target="bob")`) are used for the relationship matrix.

See [`honcho-integration/`](honcho-integration/) for peer, session, representation, and dreaming details.

## Why this topology

The Narrator is the puppeteer; the actors are the puppets-with-souls; Honcho is the thread that makes a puppet remember. Every emotionally specific moment in the game is a function of one or more actors generating output using their witness-bound representation, then the Narrator synthesizing those outputs into prose.

The architecture is the design statement. See [Pillar 2](../principles/pillars/02-characters-witness-bound.md) and [honcho-mapping](../mechanics/memory-model/honcho-mapping.md).
