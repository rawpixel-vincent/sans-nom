# Phase 2 Exit Criteria

Phase 3 opens only when all of the following hold.

## 1. Cross-session continuity is verified

Sample of 10 multi-session beta playthroughs. Each is reviewed for:

- Characters returning across sessions remember relevant prior interactions (verified by querying their representations at session boundaries).
- No "amnesia" events where a character forgets a load-bearing prior fact.
- Session boundaries land at narratively appropriate moments (location change, time skip, or natural beat — see [session boundary triggers](../../architecture/honcho-integration/sessions.md)).

Pass: ≥9 of 10 playthroughs show clean continuity. Failures get specific tickets, not blanket fixes.

## 2. Image generation pipeline ships and opt-in adoption is measurable

The image gen feature works end-to-end: player toggles on; opt-in image accompanies a scene; image style-locks to the established palette; latency is tolerable; cost is bounded.

Pass: opt-in adoption rate among the beta cohort is ≥20% (a meaningful share of players turn it on at least once); image-style consistency is graded by the designer at ≥80% match-to-reference across a sample of 50 generated images.

## 3. Generated NPC density is sustainable

Generated NPC count per playthrough does not balloon — IP flavor is preserved. Sample of 10 playthroughs: average generated NPC count per playthrough ≤30; no playthrough exceeds 50; designer graders judge the IP feel as intact across the sample.

Failure mode: too many generated NPCs makes the world feel like generic sci-fi rather than the dynasty's compound. Mitigated by Narrator card iteration if it surfaces.

## 4. Death state is implemented and reasonable

Sans-Nom can die. When he dies: the workspace closes gracefully, the player sees an in-tone end-screen (see [decisions log](../../project/decisions-log.md) — design TBD), the workspace is archived as "boys who lived" / accessible via Recall, and the player can start a fresh boy.

Pass: 100% of death events in beta close gracefully; at least one beta player who experienced death starts a fresh boy within 48 hours (signal that the death state doesn't kill engagement).

## 5. Bushitaka training arc reads in tone

The Bushitaka introduction across Phase-2 scenes is judged by the narrative designer to read in tone: the discipline is felt, not explained; the form is introduced with weight; Sans-Nom's relationship with it is shown evolving.

Pass: a curated sample of Bushitaka-arc scenes from 5 different beta playthroughs all pass designer review.

## 6. Cost model still holds

Per-turn cost stays within 25% of projection (a wider tolerance than Phase 1's 20% because image gen is a new variable). See [per-phase burn](../../project/cost/per-phase-burn.md).

## 7. ADRs closed

Any new ADRs opened during Phase 2 (likely: image-gen vendor decision, multi-session boundary policy refinements) are status `accepted`.

## What it means to fail

Same pattern as Phase 1. Phase 3 does not open. Team identifies the failed proof, scopes remediation, re-runs the proof at the next review.
