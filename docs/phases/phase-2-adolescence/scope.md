# Phase 2 — Adolescence and the Wider World

## Scope

Five months. Sans-Nom ages 12 through 17. Multiple locations. Multiple sessions. The Bushitaka training arc begins in earnest. Encounters with more of the cast — generated NPCs, glimpses of the older order (Emperor's court, Halerce-style remnants, Imperial machinery).

The slice in Phase 1 proved the architecture works in one place with three actors. Phase 2 stresses it across locations and time.

## What gets built

- Multi-session continuity. Sans-Nom leaves the compound. New sessions open. Cross-session character memory verified end-to-end. See [`architecture/honcho-integration/sessions.md`](../../architecture/honcho-integration/sessions.md).
- Bushitaka training arc — a coherent multi-scene progression introducing the form, the discipline, and (eventually) what the form is *for*. The Narrator must avoid over-explaining; the player should still feel the gravity arriving rather than being told.
- More of the cast, gradually. Generated NPCs grow in number — soldiers, traders, religious figures, members of the older order. The cap on simultaneously-active generated cast climbs to ~30 across a playthrough.
- **Opt-in scene image generation.** Phase-2 tech stretch goal. Hosted generative model (Flux Pro / Imagen / similar). Style-locked to a Gimenez-influenced palette via prompt prefix. One image per scene maximum; player-toggleable. Routed through the [Python sidecar](../../architecture/python-sidecar.md).
- **First death-state implementations.** Sans-Nom can die in this phase. Story ends. The workspace is archived; the player begins a fresh boy. See [multiple Sans-Noms](../../mechanics/player-character/multiple-sans-noms.md).
- Beta cohort grows from ~50 to ~500.

## What does NOT get built

- The Bushitaka *decision* (the kill-the-prior-Metabaron event). That's Phase 3. The player meets the gravity here; the player faces the choice in Phase 3.
- Audio narration. Phase 3.
- Monetization. Phase 3.
- The chest-crest mechanic. Phase 3.

## Why five months

Phase 2 is the riskiest phase in terms of architecture stress:

- Cross-session continuity is the main novelty. Bugs in this area silently corrupt characterization.
- Generated NPC density spikes. Quality of generated characters becomes a live concern.
- Image generation introduces a new tech surface (model selection, style consistency, cost).
- The death state is a new player-facing concept that needs careful UX work.

Five months is the floor; could extend to six if image gen quality requires it.

## Team responsibilities

- **Lead engineer** — multi-session integration testing, image gen pipeline (sidecar additions), death-state UX integration.
- **Narrative designer** — Bushitaka training arc scene authoring (the Narrator gets prompts; the corpus extends; the bar holds), continued Narrator card iteration.
- **Frontend / UX engineer** — image display + opt-in toggle, location-arc UI affordances, death-state UX (the moment the workspace closes).
- **Producer / playtest lead** — beta cohort growth, dashboard for new metrics (image-opt-in rate, multi-session continuity errors, NPC count distribution).
