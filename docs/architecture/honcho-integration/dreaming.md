# Dreaming

Honcho's background reasoning pipeline. Surfaces stable facts from message history and proposes them as card updates.

## What dreaming does

- Runs continuously over committed scene messages.
- Identifies recurring patterns, durable observations, stable character states.
- Proposes card updates (additions, replacements, merges).
- Applies the updates per Honcho's configuration (auto-apply with confidence threshold, or queue for review).

This is one of the three [card update paths](../../mechanics/memory-model/card-updates.md). The other two are Narrator-triggered and scene-end checkpoint.

## Configuration

Per peer or per workspace:

- **Auto-apply threshold** — confidence score above which dreaming-proposed updates apply without review. Default: high enough that auto-applied updates are rare in Phase 0 (everything is reviewed); relaxed in Phase 1+ as the team gains confidence.
- **Review queue** — proposed updates below the auto-apply threshold accumulate in a review queue. In dev, the team reviews via a CLI tool. In prod, low-confidence proposals are silently dropped (with telemetry).
- **Cadence** — Honcho-managed. The team does not schedule dreaming runs.

## What dreaming is good for

- Catching patterns the Narrator did not explicitly flag. ("Sans-Nom keeps mentioning the eastern hall over five scenes" — dreaming surfaces this; the Narrator may not have realized it was a pattern.)
- Capturing slow drift in a character. ("Aghora has become more guarded over the last 30 scenes" — dreaming notices the trend; updates Aghora's card to reflect.)
- Surfacing cross-peer patterns. ("Tonto and Aghora are both increasingly concerned about Sans-Nom's questions about his father" — dreaming may surface this as a fact on each peer's card.)

## What dreaming is *not* good for

- Deliberate beats. If the Narrator wants a moment to land as a stable card fact, it should fire an explicit `set_card` rather than wait for dreaming.
- Single-scene reactions. Dreaming targets *patterns*; one-off scene responses do not become card facts.
- Cross-workspace patterns. Dreaming is workspace-scoped.

## Interaction with [card eviction](../../mechanics/memory-model/card-eviction.md)

When a dreaming-proposed update would push a card past the 40-fact cap, the curation pass runs. The proposed update is treated as a candidate alongside any in-flight Narrator-triggered update; the curation pass picks the best resulting card state.

## Cost

Dreaming runs in Honcho's pipeline, billed as message-reasoning cost. See [per-turn cost model](../../project/cost/per-turn-model.md) — the per-message reasoning cost (~$0.005 per message) includes dreaming-pipeline overhead.

## Telemetry

- Dreaming-proposed update count per session.
- Dreaming auto-apply rate.
- Review-queue depth (in dev).
- Card-update events (all paths) emit telemetry; see [observability.md](../observability.md).
