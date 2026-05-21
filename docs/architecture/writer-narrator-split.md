# Writer / Narrator Split

Sans-Nom separates story structure from player-facing prose.

The Writer decides what can happen next. The Narrator renders the selected
moment. This keeps branching, validation, and lore fidelity out of the prose
model's immediate burden while keeping final text in one voice.

## Writer

The Writer is the slower planner. It produces and refreshes bounded
`StoryPacket` material:

- `StoryBible` updates and continuity notes.
- Packet nodes, gates, choices, terminal handoff notes, and memory intents.
- Actor needs, when a node truly requires actor support.
- Lore-fidelity obligations from the active lorepack scenario contract.

In scenario-contract modes, the Writer does not invent the central premise. It
stages and branches from the active lorepack's authored scenario contract.

For seeds, the accepted runway remains compact:

- one intro/onboarding turn;
- one real decision turn;
- three third-turn branch nodes, one for each real decision.

For runtime refresh, the Writer continues from current canon, the player
decision, current arc position, active contract, and relevant story state.

## Narrator

The Narrator is the foreground prose layer. It receives:

- the current story state and continuity context;
- one selected packet node;
- lore-fidelity scene constraints;
- scoped actor output when actor needs are present and supported.

The Narrator writes the player-facing scene, presents the packet's choices, and
prepares commit material. It may not silently invent new choice targets or drift
away from accepted packet structure.

## Advisory Roles

Scenarist and LoreExpert are advisory:

- Scenarist provides one focused brainstorm/critique/staging pass at a time.
- LoreExpert acts as lore authority/reference support for fidelity checks.

Neither role owns final story authorship. The Writer owns structure; the
Narrator owns prose.

## Council Reference Boundary

Council of Castaka remains a useful reference for lore encoding and agent
patterns, not a mechanics template. Sans-Nom does not default to dynasty
roundtables, all-character fanout, or resurrected council behavior.

## Filesystem Boundary

Writer and Narrator read tools operate inside scoped agent workspaces. They may
read staged `story/`, `scenarists/`, `workshop/`, and `lorepack/` files when
available. They should not see repo-root `cmd/`, `internal/`, old stories, old
seed runs, or raw `lore/packs/...` paths.

See [agent-filesystem](agent-filesystem.md).

## Memory Boundary

Honcho receives only rolled-out canon:

- player decision;
- final Narrator prose;
- surfaced actor speech/actions;
- scene metadata;
- directional witness lists.

Writer drafts, unused branches, Scenarist material, and actor private interiors
are not committed to shared scene memory. If actor-private evolution is needed,
it must use a private actor-scoped channel that does not become witnessed canon
for other peers.
