# Sans-Nom Card Evolution

The seed card grows into a specific boy across a playthrough. The card is the player's character growing up.

## Example trajectory

After 30 scenes, the card might include:

> `"Sans-Nom is wary of Honorata's portraits in the eastern hall."`

After 100 scenes:

> `"Sans-Nom has begun training with Bushitaka stances. Earth comes naturally; Sky is harder."`

After 200 scenes:

> `"Sans-Nom has begun to suspect Aghora's grief is connected to Steelhead's stasis."`

These are illustrative, not scripted. Different playthroughs evolve into different cards.

## How evolution happens

Through three paths:

1. **[Honcho dreaming](../../architecture/honcho-integration/dreaming.md)** — automatic, background. Picks up stable facts from conversations.
2. **Narrator-triggered manual updates** — when the Narrator explicitly decides "this is now a stable fact about Sans-Nom," it calls `set_card` to append.
3. **Scene-end checkpoint** — end-of-session ritual: the Narrator reviews the session and surfaces candidate card updates for review (in dev) or auto-commit (in prod with a quality threshold).

See [card updates](../memory-model/card-updates.md) for the full pipeline. Card cap and eviction policy: [card eviction](../memory-model/card-eviction.md).

## Why the card *is* the saved game

In a structural sense, the Honcho session and its representations *are* the save file. There is no separate save state. The boy you have grown is the boy you reload.

This means save/load operates on workspaces, not slots. See [multiple Sans-Noms](multiple-sans-noms.md).

## What the Narrator should *not* commit to the card

- Single-scene reactions ("Sans-Nom was afraid in this moment"). Reactions are scenes; only when something stabilizes ("Sans-Nom is afraid of Tonto's stories about Edna") does it become a card fact.
- Other characters' opinions of Sans-Nom. Those go on *their* directional card-for-Sans-Nom, not on his card. See [representations](../../architecture/honcho-integration/representations.md).
- Spoilers about Sans-Nom's future. The card describes who he is, not who he will become.
