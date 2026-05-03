# Card Eviction Policy

The 40-fact-per-card cap will bite in long playthroughs. This file is the game-design view of how the cap is managed. The architecture-side decision is captured in [ADR 0004](../../architecture/decisions/0004-card-eviction-policy.md).

This was an open question in the original draft (Appendix C.8 #4), now resolved.

## The policy

**Narrator-curated, with most-recent fallback.**

When a card update would push past 40 facts, the Narrator runs a curation pass:

1. Read the existing 40 facts plus the candidate addition.
2. Decide which existing fact (if any) should be replaced or merged with the addition.
3. If the curation pass cannot identify a clean replacement (e.g., the new fact is genuinely additive, no existing fact is stale), fall back to evicting the *least-recently-referenced* fact.

The most-recent fallback is the safety net, not the primary path. The Narrator should be doing real curation — merging redundant facts, retiring stale ones, sharpening fact text — not just dropping the oldest entry.

## Why this policy

- **It honors the original GDD's stance** that the Narrator is responsible for keeping cards lean (§9: *"The Narrator is responsible for keeping cards lean by replacing rather than appending when possible."*). The policy is the operationalization of that stance.
- **It avoids time-sensitive pure FIFO**, which would silently lose load-bearing early facts. (Sans-Nom's seed trait *"I am called Sans-Nom because no one has named me"* must not get evicted at scene 200.)
- **It avoids reference-counting oracles**, which would require side-channel tracking of which facts the Narrator drew on per scene. The most-recent-referenced heuristic is cheap; tracking it is one timestamp per fact.

## What the policy rules out

- **Pure most-recent-write FIFO.** Loses seed traits.
- **Pure reference-count LRU.** Hard to implement without instrumentation drift.
- **Manual curation only.** Cards would silently grow beyond cap and lose facts unpredictably.
- **Higher cap.** Would defer the problem; would also degrade Honcho's reasoning quality on the cards.

## Implementation hooks

- The reference timestamp is updated whenever the Narrator or an actor agent reads the card. Reads come through the [Python sidecar's `/v1/peer/{id}/card` endpoint](../../architecture/python-sidecar.md).
- The curation pass is a Narrator subroutine, called inline from the [card-update path](card-updates.md). It uses Sonnet (cheap) rather than Opus.
- Eviction events are logged with diff for replay (see [observability](../../architecture/observability.md)).

## Review trigger

Revisit this policy if:

- Beta playthroughs show >5% of cards hitting cap before scene 200.
- The Narrator's curation passes show consistently bad merges (e.g., merging facts that should have stayed distinct).
- Honcho's own reasoning over near-cap cards degrades measurably.
