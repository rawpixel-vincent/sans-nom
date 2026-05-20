# Actor Card Seeding

Actor cards are the baseline identity material for callable character agents.
They are distinct from lorepack scenario contracts and from Honcho's evolved
runtime representations.

## Seed Sources

Current accepted actor cards live under the selected lorepack when available,
for example:

```text
lore/packs/<pack_id>/comics/peer-cards/cards/accepted/<peer_id>.json
```

The active pack may be staged into model-visible workspaces as `lorepack/`, but
actor-card loading is application-owned. Creative models should not crawl raw
repo lore paths to discover cards.

## Bootstrap Boundary

When a story workspace initializes, the runtime can create Honcho peers and seed
cards from accepted pack artifacts. If a pack does not contain a supported actor
card, that actor is not callable yet.

Initial seed packets currently avoid actor needs unless valid actor cards exist.
This keeps seed generation from naming actors the runtime cannot call.

## Seed/Evolution Boundary

The seed card is immutable for a workspace. Runtime evolution happens through
Honcho cards and directional representations. Editing a source card affects new
workspaces, not already-evolved playthroughs.

## Generated Or Missing Actors

If a packet asks for an actor without a Honcho/local card, the runtime should
skip or reject that actor need instead of crashing. A new actor entering a scene
also creates a new scene/session boundary so the actor does not inherit prior
events they did not witness.

## What Gets Versioned

- Source card artifacts under `lore/packs/<pack_id>/...`.
- Tests that prove actor needs are pruned or rejected when unsupported.

Per-workspace evolved card state lives in Honcho and story runtime artifacts.
