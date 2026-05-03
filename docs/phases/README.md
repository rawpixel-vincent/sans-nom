# Phases

The WHEN of sans-nom. Each phase is a contained chunk of work with clear scope, deliverables, and exit criteria. Phases are gated — Phase N+1 only opens after Phase N's exit criteria pass.

## Phase ordering

| Phase | Months | Subtree | One-line scope |
|---|---|---|---|
| 0 — Pre-production | 1 | [`phase-0-preproduction/`](phase-0-preproduction/) | Lock the Narrator's voice, stand up Honcho, build the CLI testbed, seed actor cards |
| 1 — Vertical slice | 2–4 | [`phase-1-vertical-slice/`](phase-1-vertical-slice/) | Sans-Nom ages 6–12 at Aghora's compound, ~80 scenes, free public beta |
| 2 — Adolescence | 5–9 | [`phase-2-adolescence/`](phase-2-adolescence/) | Bushitaka training arc, multiple locations, opt-in image generation, first deaths |
| 3 — Dynasty's weight | 10–14 | [`phase-3-dynasty-weight/`](phase-3-dynasty-weight/) | Chest-crest mechanic, Aghora-mercy revelation, opt-in TTS, v1.0 launch |

## Exit criteria pattern

Every phase has an `exit-criteria.md` listing the named conditions for the next phase to open. The pattern: a small set of falsifiable checks, each tied to a deliverable or a measured behavior. No phase exits on schedule alone; it exits on criteria met.

## What changes between phases

- **Phase 0 → 1**: shape of the Narrator's voice is locked; Honcho integration verified end-to-end; CLI lets devs play through their own builds.
- **Phase 1 → 2**: the seven slice proofs (see [`phase-1-vertical-slice/exit-criteria.md`](phase-1-vertical-slice/exit-criteria.md)) hold across cohort beta data. Authoring loop has survived 50+ playthroughs without tonal drift.
- **Phase 2 → 3**: image generation pipeline ships and opt-in adoption is measurable; multi-location continuity tested; first death-state implementations stable.
- **Phase 3 → v1.0**: monetization model selected and load-tested; chest-crest mechanic prose-verified against canon.
