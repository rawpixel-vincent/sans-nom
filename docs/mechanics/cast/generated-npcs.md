# Generated NPCs

Characters the Narrator invents during play. Each becomes a peer in Honcho on first appearance and persists thereafter.

## When the Narrator generates a character

- A scene calls for a non-canonical presence (a servant in the kitchen, a merchant in the gardens of Flos, a soldier on the road).
- The player's free-text input introduces an entity the world should now have (the player asks Tonto about a Berard noblewoman from his early career — the Narrator may invent a specific person if no canonical one fits).
- A side-arc is opening that needs a recurring face.

## What the Narrator must do at first appearance

1. **Decide the character is going to be persistent.** Disposable presences (a passing crowd) do not get peers. Anyone the Narrator might want to reference again does.
2. **Generate a peer card** seeded from the scene that produced them. Card includes: name, age, role, a few traits, and one specific witnessed moment from the scene of their introduction.
3. **Persist the peer in Honcho** so subsequent scenes can call them as actors.
4. **Tag the peer as `generated`** so analytics and audit passes can distinguish generated from canonical characters.

## Constraints on generated characters

- Generated characters cannot violate canon. A generated character cannot, for example, possess Shabda-Oud powers (those are bounded by the priestess line). The Narrator's [peer card](../../architecture/narrator-agent/peer-card.md) enforces this authorially.
- Generated characters cannot rewrite known facts about canonical characters. They can have *opinions* of canonical characters (their representations of, e.g., Aghora are valid Honcho objects) — those opinions are the generated character's perspective, not new canon.
- Generated characters can die, leave, or be discarded. Dead generated NPCs become absent peers (see [dead-and-absent.md](dead-and-absent.md)) just like dead canonical characters — their representations of others persist in Honcho, queryable later.

## Lifecycle in the Honcho workspace

- Created on first appearance.
- Updated through normal [card update paths](../memory-model/card-updates.md) when present in subsequent scenes.
- Subject to the same [card eviction policy](../memory-model/card-eviction.md) as canonical characters — the Narrator curates, with most-recent fallback.
- Never imported across workspaces. Each playthrough's generated cast is its own.

## Analytics

The proportion of generated to canonical character interactions is a Phase-1 instrumentation point. Too many generated characters early can dilute the IP's flavor; too few can make the world feel sparse. See [observability](../../architecture/observability.md).
