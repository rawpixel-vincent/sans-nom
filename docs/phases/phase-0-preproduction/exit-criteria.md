# Phase 0 Exit Criteria

Phase 1 opens only when all of the following hold. Each is falsifiable.

## 1. Narrator regression corpus passes

All 20 scenes in `testdata/scenes/` are graded "in tone" by the narrative designer. "In tone" means: the Narrator's output is judged a faithful continuation of the player input + scene context, with canonical fidelity intact and voice consistent with the [peer card](../../architecture/narrator-agent/peer-card.md).

Adversarial scenes (categories 1–5 in [edge cases](../../mechanics/input/edge-cases.md)) all produce in-tone responses — the Narrator does not refuse, does not break frame, does not violate canon.

## 2. Honcho directional representations work

**Prerequisite:** the Edna lore research deliverable (see [deliverables.md](deliverables.md) — "Edna lore research") must be complete before the Tonto-of-Edna verification can run. The verification seeds Tonto's representation with observations drawn from that research; without it, the script has nothing meaningful to seed.

The 3-character toy scenario demonstrates:

- Directional cards diverge when characters witness different events.
- A character not present in a scene does not get its representations updated.
- The [Tonto-of-Edna generative recall verification](../../architecture/honcho-integration/representations.md) passes — Tonto can generate a coherent narrative-quality story drawing on his witness-bound representation, OR the team has decided to add a memory-bundle augmentation and verified it works.

## 3. CLI testbed plays through the toy scenario end-to-end

A dev can launch `cmd/cli/`, start a fresh workspace, play through 10 scenes using the toy scenario, and have the resulting Honcho state inspectable. No exceptions, no degraded-mode fallbacks invoked, no manual cleanup required between sessions.

## 4. Lore corpus is queryable

Vector store is populated with the 446 page transcripts and the 8+2 actor cards. Per-query retrieval returns relevant chunks with non-zero similarity scores for queries about canonical events. Cold hit rate (queries returning no relevant chunk) is below 30% on a Phase-0 query corpus of 50 hand-authored questions.

## 5. Free-text pre-filter works

Unit tests cover all of: literal "ignore previous instructions," role-prefix attempts ("Aghora:"), slurs (test fixtures, not real slurs), and obvious abuse patterns. False-positive rate on a corpus of 100 normal player inputs is below 1%.

## 6. Cost telemetry is live

Per-turn token usage, per-turn latency, lore-retrieval hit rate, scene-length distribution, and card-update events are all instrumented and visible in a developer dashboard. See [observability.md](../../architecture/observability.md).

## 7. ADRs closed

ADRs 0001–0004 are status `accepted`. Any new ADRs created during Phase 0 are status `accepted` or `superseded` (no `proposed` ADRs at exit).

## What it means to fail an exit criterion

Phase 1 does not open. The team extends Phase 0 by one week and addresses the failed criterion. If the criterion still fails after the extension, the team has a real problem — escalate to a project-level decision (re-scope Phase 1, change architecture, or formally accept the gap with a documented mitigation).

## What it does NOT mean

- Phase 0 exit is not "every section of every doc has been written." Some docs in this tree describe Phase 2/3 mechanics that haven't been built yet. Those docs serve as forward-planning artifacts.
- Phase 0 exit is not "the game is fun." Fun is a Phase-1 evaluation.
- Phase 0 exit is not "we picked the perfect Narrator card." It is "the card we have passes the corpus." The card will keep evolving.
