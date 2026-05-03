# Phase 3 — The Dynasty's Weight

## Scope

Five months. Sans-Nom approaches the Bushitaka decision (or doesn't — by design, no fixed outcome). Long-arc continuity tested. The Aghora-mercy revelation. The chest-crest inheritance event. Steelhead encounter (live or via stasis depending on player path). Optional audio narration.

Phase 3 is the game's full shape. v1.0 launches at the end.

## What gets built

- **The chest-crest mechanic** — Sans-Nom inherits the crest. Ancestor peers' representations of him become readable to him through the crest. He can in some sense converse with them. See [dead-and-absent.md](../../mechanics/cast/dead-and-absent.md) and [Pillar 2](../../principles/pillars/02-characters-witness-bound.md).
- **The Aghora-mercy revelation arc.** Sans-Nom learns what his parent did. The Bushitaka decision begins to land on him.
- **Steelhead encounter.** Live, via stasis, via chest-crest, or never — depends on player path.
- **Apparitions** (Aghnar's psychic legacy). The Narrator can invoke this when in tone. See [Aghnar cast file](../../mechanics/cast/aghnar.md).
- **Optional audio narration.** TTS via [ElevenLabs](../../architecture/python-sidecar.md). Per-character voices. Opt-in. Phase-3 stretch.
- **v1.0 launch.** Paid version with one of the [monetization options](../../project/cost/monetization-options.md).
- **Long-arc continuity stress test** — playthroughs spanning 20+ hours and 5+ sessions verified end-to-end.

## What does NOT get built

- New mechanics beyond v1.0 scope. Family-tree views, multiplayer Sans-Noms, additional IP adaptations — all are post-v1.0.

## Why five months

- Month 1: chest-crest mechanic implementation + Aghora-mercy revelation arc design.
- Month 2: Steelhead encounter integration + apparition mechanic.
- Month 3: TTS pipeline (sidecar additions, voice selection per character).
- Month 4: monetization implementation (whichever model is selected at the start of Phase 3).
- Month 5: v1.0 polish, launch readiness, marketing rhythm, public launch.

Could compress to four months if monetization is simple (BYOK only) or extend if TTS quality requires it.

## Team responsibilities

- **Lead engineer** — chest-crest implementation (the architectural exception to witness-bound — see [Pillar 2](../../principles/pillars/02-characters-witness-bound.md)), TTS pipeline integration, monetization flow.
- **Narrative designer** — chest-crest scenes (each ancestor speaking *to* Sans-Nom is a new authoring challenge), Aghora-mercy revelation arc, Steelhead-encounter variations.
- **Frontend / UX engineer** — TTS opt-in toggle, audio playback UI, monetization checkout flow (or BYOK key entry).
- **Producer / playtest lead** — v1.0 launch readiness, beta-to-prod cohort migration, marketing campaign, monetization model evaluation in beta.
