# Phase 0 — Pre-production

## Scope

One month of foundational work. The voice of the Narrator is the game; this phase locks it. Honcho is the memory substrate; this phase verifies it. The actor cards are the dynasty's seeds; this phase formats them.

No player-facing UI. No web frontend. The only consumer of the system is the team's CLI testbed.

## What gets built

- The Narrator's [peer card](../../architecture/narrator-agent/peer-card.md), v1, validated against 20 hand-authored test scenes (the [regression corpus](deliverables.md)).
- The Honcho integration end-to-end. A 3-character toy scenario verifies directional-representation behavior. The [Tonto-of-Edna generative recall verification](../../architecture/honcho-integration/representations.md) passes or surfaces the need for a memory-bundle augmentation.
- The Bubble Tea CLI testbed (`cmd/cli/`). Devs play through their own builds at terminal speed. Adapted from Council of Castaka patterns.
- The [actor card seeding](../../architecture/actor-agents/card-seeding.md) — all 8 canonical character cards copied, lightly edited for game use; Tonto and Lothar cards written from scratch.
- The [comic corpus ingestion](../../architecture/lore-retrieval.md) — 446 pages OCR'd, manually reviewed for canonical-quote accuracy, embedded into the vector store.
- The thin [free-text pre-filter](../../mechanics/input/edge-cases.md) for genuine abuse and obvious prompt-injection patterns.

## What does NOT get built

- Any web frontend. That's Phase 1.
- Image generation. Phase 2.
- Audio narration. Phase 3.
- Monetization. Phase 3.
- Generated NPCs (the mechanism is *designed* but not exercised; the slice in Phase 1 introduces the first ones).
- Multi-session continuity exercise (Phase 1 vertical slice is one session).

## Why one month

This is the *only* phase where the team is allowed to over-spend on prompt iteration. The Narrator's voice is the load-bearing artifact; if it isn't right, nothing downstream can save the game. Discipline: time-box prompt iteration to working hours; set a quality bar (the regression corpus passes); stop fiddling once the bar is met.

If at the end of one month the corpus does not pass, extend Phase 0 by one week and re-evaluate. Do not move to Phase 1 with a wobbly Narrator.

## Team responsibilities

See [`project/team/`](../../project/team/) for full role descriptions. Phase-0 specifics:

- **Lead engineer** — Honcho integration, agent loop scaffolding in Go, Python sidecar, lore retrieval pipeline.
- **Narrative designer** — Narrator peer card, the 20 hand-authored test scenes, actor card seeding, manual review of comic OCR.
- **Frontend / UX engineer** — not on critical path this phase; can begin scaffolding the Phase-1 web UI in parallel, but doesn't need to ship it.
- **Producer / playtest lead** — sets up cost monitoring, observability dashboards, schedules the Phase-0 → Phase-1 review.
