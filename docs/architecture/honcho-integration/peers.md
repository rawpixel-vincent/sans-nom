# Peers

Honcho's first primitive. One peer per character — including the player as `sans-nom`.

## Peer creation

- **At workspace bootstrap.** The `seed-honcho` script creates one peer per canonical character + Tonto + Lothar + the player. See [card-seeding.md](../actor-agents/card-seeding.md).
- **At first appearance (generated NPCs).** The Narrator creates the peer when a generated character first becomes addressable. See [generated-npcs](../../mechanics/cast/generated-npcs.md).

## Peer types

| Peer | Purpose | Always queryable as actor? |
|---|---|---|
| `sans-nom` | The player. Has a card the Narrator updates. | No — the player is acted by the human, not by an agent |
| Canonical character peers (`aghora`, `tonto`, etc.) | Active actors. Queryable in scenes where they are present. | Yes (scene-bound) |
| Absent canonical peers (`vicenta`, `oda`, etc.) | Reference peers. Their cards inform other characters' representations. | Only via channels in [dead-and-absent](../../mechanics/cast/dead-and-absent.md) |
| Generated NPC peers | Same as canonical, but persisted only in this workspace. | Yes (scene-bound) |
| `narrator` | The Narrator's own peer card. Optional — see ADR-pending question. | Special — not invoked as an actor; queried for its own representations of others |

## The `narrator` peer

Treating the Narrator as a Honcho peer (so it has its own representations of all characters) is the recommended approach for Phase 1. It gives the [audit pass](../narrator-agent/failure-modes.md) a place to read from, and makes the Narrator's own evolving understanding of *this Sans-Nom* a first-class object.

The original draft flagged this as an open question (Appendix C.8 #5). Decision: **Narrator-as-peer in Phase 1.** Re-evaluate if the cost or complexity becomes a problem.

## Peer ID conventions

- All-lowercase, hyphen-separated.
- Canonical character peers use the character's most-common short name: `aghora`, `othon`, `vicenta` (not `dona-vicenta-gabriela-de-rokha` — too long).
- The player is always `sans-nom` (with hyphen).
- Generated NPC peers use a `gen-` prefix: `gen-merchant-flos-1`, `gen-soldier-road-7`.
- The Narrator is `narrator`.

## What peers are *not*

- They are not user accounts. The player has zero or more workspaces; each workspace has one `sans-nom` peer. Account/auth is a separate concern handled at the web frontend layer.
- They are not character classes or templates. Each peer is a specific entity with a specific card.
- They are not addressable across workspaces. Aghora in workspace A is a different peer object from Aghora in workspace B (with the same seed but separate evolution).
