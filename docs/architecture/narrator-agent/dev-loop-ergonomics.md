# Dev-Loop Ergonomics

How the narrative designer iterates on the Narrator's voice without redeploying the server. This was missing from the original draft; specified here.

## Narrator role doctrine

The old root `lore/narrator/` markdown card has been removed. Narrator doctrine is assembled from runtime prompts, the active Story Bible, witness rules, and seeded Honcho role cards.

## Hot reload in dev

The Go server watches story state and pack/index artifacts that affect context assembly. When a runtime prompt or seeded role card changes:

- The Narrator's prompt assembly picks up the new card on the *next* turn.
- In-flight turns finish with the old card (no mid-turn switching).
- A telemetry event is emitted: `narrator.card.reloaded` with a hash of the new card content.

## In-prod card revisions

Production deployment ships the card baked into the Docker image. Card revisions are deploys. This is intentional — production card changes affect every player and should go through the regression corpus first. See [Phase 0 deliverables](../../phases/phase-0-preproduction/deliverables.md) for the corpus spec.

## CLI/TUI testbed for fast iteration

`cmd/cli/` is the primary local testbed. Normal human play uses the full-screen
Bubble Tea TUI; raw output remains for scripted/debug paths. It can:

- Replay a regression scene from the corpus with the current Narrator card.
- Diff the Narrator's output against the corpus's reference output.
- Step through manual play with narration, choices, free text, and live status.
- Run scripted/debug flows in raw mode for tests and traces.
- Run the full corpus with a summary pass/fail.

This is the designer's primary workspace during Phase 0. Bubble Tea is reused from Council of Castaka — see `external/references/council of castaka/` Go source for the patterns.

## Lorepack and retrieval iteration

Lorepack contract changes are picked up by new seed/runtime context assembly.
The active pack is staged as `lorepack/` inside model-visible agent workspaces.
Future vector-indexed corpus changes can hot-reload under the same contract
boundary. See [lore-retrieval.md](../lore-retrieval.md) and
[agent-filesystem.md](../agent-filesystem.md).

## Actor card iteration

Actor cards live in accepted peer-card artifacts under the selected lore pack, for example `lore/packs/<pack_id>/comics/peer-cards/cards/accepted/<peer_id>.json`. In-Honcho evolution is a separate channel; editing a base artifact does not retroactively change a workspace's evolved actor representation.

## What this enables

- Designer can iterate on the Narrator's voice without bothering the engineer.
- Designer can test changes against the regression corpus *before* committing — fast feedback loop.
- Engineer can focus on the loop architecture without being interrupted for prompt tweaks.

## What this rules out

- Live-edited cards in production. Deploy is the cadence in prod, intentionally.
- Per-player card customization. The Narrator is one Narrator per build. Multi-tenancy is out of scope.
