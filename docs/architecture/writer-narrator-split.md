# Writer / Narrator Split

Phase 0 uses two authoring layers.

## Creative Writer

The Creative Writer is the slower background planner. It produces a bounded
`StoryPacket`: a structured decision tree for the current chunk, normally 3-5
scene nodes. The packet contains beats, gates, choices, actor needs, memory
intents, and revision notes.

The packet is not canon. It is planning material.

## Narrator

The Narrator is the foreground layer. It receives one selected packet node,
current context, and scoped actor output. It writes the player-facing prose,
presents choices, and prepares the Honcho commit.

The Narrator owns final prose. The Creative Writer owns structure.

## Council Reference Boundary

Council of Castaka is a lore-encoding reference, not a mechanics template for
Sans-Nom. The game does not default to a dynasty roundtable, all-character
fanout, or resurrected council behavior. Actor cards are grounding material and
selective scene inputs.

## Memory Boundary

Honcho receives only rolled-out canon:

- player decision
- final Narrator prose
- surfaced actor speech/actions
- scene metadata
- directional witness lists

Writer drafts, unused branches, and actor private interiors are not committed to
shared scene memory. If actor-private evolution is needed, it must use a private
actor-scoped channel that does not become witnessed canon for other peers.

