# Phase 1 — Vertical Slice

## Scope

Three months. The first player-facing game. A web-based, prose-first build of "Childhood at Aghora's compound" — Sans-Nom ages 6 through 12, one location, one Honcho session, ~80 plausible scenes per playthrough.

For the detailed slice spec (cast, locations, beats, what gets proven), see [slice-spec.md](slice-spec.md). For the exit criteria, see [exit-criteria.md](exit-criteria.md).

## What gets built

- Web frontend (React or Svelte + Tailwind, designer's pick) with prose pane, choice list, free-text input, scrollback, Tonto-as-codex sidebar.
- Server-side agent loop in production-readiness — streaming Narrator output via WebSocket, scene commit to Honcho, telemetry from day one.
- Authentication / account binding (so cross-device save-resume works via Honcho's hosted account-bound workspaces).
- Workspace lifecycle UI: new game, resume, view "boys who lived" (archived workspaces).
- All actor cards from Phase 0 in production use (Aghora, Tonto, Lothar live; the absent-cast informs Tonto's stories).
- The first generated NPCs (kitchen presences, garden visitors, ambient figures the Narrator invents).
- Closed beta cohort management. ~50 testers. Per-tester turn cap to bound cost.
- Cost dashboard, scene-quality auto-grader, drift-flag alerting in production.
- **Sonnet-Narrator A/B test.** Mid-beta (Month 3), run the regression corpus against a Sonnet-tier Narrator in place of Opus. Compare output on 5 key scenes from the corpus. Document the quality-vs-cost trade-off. This data feeds Phase 2 cost model calibration — if Sonnet passes at acceptable quality, per-turn cost halves (~$0.035 vs. ~$0.07). See [per-turn cost model](../../project/cost/per-turn-model.md).

## What does NOT get built

- Image generation. Phase 2.
- Audio narration. Phase 3.
- Multi-location continuity (Sans-Nom does not leave the compound in this slice).
- Multi-session boundary handling (the entire slice is one session).
- The Bushitaka decision arc (Sans-Nom is too young).
- Death state. (Sans-Nom does not die in the slice. Phase 2 introduces death.)
- Monetization. Beta is free.

## Why three months

- Month 1: agent loop production-hardening, frontend MVP, end-to-end integration tested with Phase-0 cast.
- Month 2: closed alpha to internal team + 5 trusted external playtesters. Iterate on Narrator card based on real playthrough data.
- Month 3: open closed beta to ~50 testers. Monitor, iterate, gather data for Phase 2 greenlight decision.

## Team responsibilities

- **Lead engineer** — production-hardening the agent loop, WebSocket streaming, Honcho production integration, telemetry pipeline.
- **Narrative designer** — Narrator card iteration based on playthrough data, regression corpus expansion as new failure modes surface, beta-tester scene review.
- **Frontend / UX engineer** — web UI, scrollback, choice + free-text input affordances, workspace lifecycle UI.
- **Producer / playtest lead** — beta cohort recruitment + management, turn-cap enforcement, weekly playtest review, marketing rhythm (devlogs, beta sign-up funnel).
