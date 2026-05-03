# Card Updates

The three paths through which peer cards (player and actors) evolve during play.

## Path 1 — Honcho dreaming (automatic)

Background pipeline. Picks up stable facts from conversations. Runs continuously over committed scene messages, surfacing recurring patterns, durable observations, and stable character states.

Use case: capturing patterns the Narrator did not explicitly flag. ("Sans-Nom keeps mentioning the eastern hall. Add to his card: he is preoccupied with the eastern hall.")

Configuration and cost: see [`architecture/honcho-integration/dreaming.md`](../../architecture/honcho-integration/dreaming.md).

## Path 2 — Narrator-triggered manual updates

When the Narrator explicitly decides "this is now a stable fact about the character," it calls `set_card` to append. This is the deliberate authoring path.

Use case: a moment the Narrator wants to *guarantee* lands as a stable trait. ("Sans-Nom has now seen Steelhead's stasis pod and chose not to ask about it. Add to his card: he keeps secrets from Aghora.")

The Narrator is the only agent allowed to trigger these updates. Actor agents cannot self-update; they evolve only through dreaming and Narrator-triggered changes.

## Path 3 — Scene-end checkpoint

End-of-session ritual. The Narrator reviews the session and surfaces candidate card updates for review (in dev) or auto-commit (in prod with a quality threshold).

Use case: bundling a session's worth of accumulated change into deliberate card updates. The dreaming pipeline catches what's recurring; the checkpoint captures what's *significant*.

In dev, these updates are presented to the team for review. In prod, they auto-commit if their confidence score (from the Narrator's own self-assessment) is above threshold; otherwise they are logged for human review.

## Card cap

40 facts per directional pair. This is a Honcho constraint. When the cap is hit, the [card eviction policy](card-eviction.md) decides what to drop.

## What never updates a card

- Player free-text input directly. The player does not write into Sans-Nom's card. Their input becomes a scene; the scene may, downstream, become a card update.
- Other characters' opinions. Aghora cannot edit Sans-Nom's card. She can only update her own *representation of* Sans-Nom (her directional card-for-Sans-Nom).
- The Narrator's commentary. The Narrator updates cards through the three paths above, not through asides.
