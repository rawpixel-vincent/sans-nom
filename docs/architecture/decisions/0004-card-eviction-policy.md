# ADR 0004 — Card eviction: Narrator-curated with most-recent fallback

## Status

**Accepted.** 2026-05-03.

## Context

Honcho enforces a 40-fact-per-card cap. The original draft flagged this in §9 ("Card limit is 40 facts per directional pair") and deferred the eviction policy to an open question (Appendix C.8 #4). This ADR resolves it.

The cap will bite in long playthroughs (200+ scenes). When a card is full and a new fact arrives, *something* has to give.

Options considered:

- **A. Pure FIFO (most-recent-write).** Simple. Loses load-bearing seed traits silently.
- **B. Pure LRU by reference count.** Theoretically correct. Requires reference-count tracking on every card read; instrumentation overhead and timing-sensitive bugs.
- **C. Manual curation only.** Card silently grows past cap if curator misses; quality drops unpredictably.
- **D. Narrator-curated with most-recent fallback.** Narrator runs a curation pass when an update would push past cap; if no clean replacement is found, evict the least-recently-referenced fact.

## Decision

**Choose D — Narrator-curated with most-recent fallback.**

The Narrator already authors the card-update path (it's the trigger for explicit `set_card` calls). Adding a curation subroutine to that path is a small extension. The most-recent fallback is the safety net for cases where curation can't find a clean replacement.

See [`mechanics/memory-model/card-eviction.md`](../../mechanics/memory-model/card-eviction.md) for the policy in plain terms.

## Consequences

### Positive

- Honors the original GDD stance ("the Narrator is responsible for keeping cards lean by replacing rather than appending when possible").
- Avoids silent loss of seed traits (pure FIFO's failure).
- Avoids reference-count instrumentation timing bugs (pure LRU's failure).
- Curation pass is cheap (Sonnet, not Opus) — see implementation hooks below.

### Negative

- Adds a Narrator subroutine on the card-update path. Marginal latency.
- Curation pass can make bad merges (e.g., merging two facts that should have stayed distinct). Mitigated by reviewing in dev; in prod, eviction events are logged with diff for replay (see [observability.md](../observability.md)).
- The reference timestamp for the most-recent fallback requires per-card-read instrumentation — timestamp updated on every card read by the Narrator or any actor.

### Implementation hooks

- The reference timestamp is updated on `/v1/peer/{id}/card` read (the [Python sidecar endpoint](../python-sidecar.md)).
- The curation pass is a Narrator subroutine called inline from the card-update path. Uses Sonnet.
- Eviction events emit `card.eviction` telemetry with `peer_id`, `target_id`, `evicted_fact`, `replacement_or_added_fact`, `policy_branch` (curated | fallback).

## Re-evaluation

Re-open this ADR if:

- Beta playthroughs show >5% of cards hitting cap before scene 200.
- Curation pass shows persistent bad merges in dev review.
- Honcho's reasoning over near-cap cards degrades measurably (separate from eviction quality).
