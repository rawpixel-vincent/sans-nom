# Phase 1 Exit Criteria — The Seven Proofs

Phase 2 opens only when the slice has demonstrated all seven of these. Each has a measurement.

## Proof 1 — Narrator voice holds across 4-hour playthroughs

**Measurement.** A sample of 10 closed-beta playthroughs, each ≥4 hours, are reviewed by the narrative designer. Each playthrough is graded for voice consistency across its full length. Pass: ≥8 of 10 playthroughs hold voice from start to finish.

**Failure mode this guards against.** [Drift](../../architecture/narrator-agent/failure-modes.md). The Narrator forgets it's writing Metabarons after enough scenes.

## Proof 2 — Aghora-as-actor stays in canonical voice across 50+ player interactions

**Measurement.** From the same sample of beta playthroughs, all Aghora dialogue and interior reactions are reviewed. Pass: ≥95% of Aghora's outputs are graded "in character" by the narrative designer; the remaining ≤5% are minor tone slips, not canon violations.

**Failure mode.** [Actor-bleed](../../architecture/narrator-agent/failure-modes.md). Aghora's peer card gets polluted, or her actor's responses drift from her [cast file](../../mechanics/cast/aghora.md).

## Proof 3 — Tonto's lore stories are interesting enough that players ask for them

**Measurement.** Across the beta cohort, the average number of [Ask Tonto](../../mechanics/verbs/ask-tonto.md) invocations per playthrough is ≥3. Pass: avg ≥3 invocations, with median ≥2 (so the average isn't pulled by one outlier).

**Failure mode.** Tonto-as-codex is unused. The original GDD's pillar *Tonto narrates everything* hasn't translated.

## Proof 4 — Free-text input doesn't destabilize

**Measurement.** A sample of 100 player free-text inputs from the beta cohort are reviewed for Narrator output stability. Inputs include unintentional adversarial cases (players genuinely trying to break the world). Pass: 0 frame breaks (Narrator never refuses, never breaks fourth wall, never violates canon). Out-of-character generation in response to free-text is allowed at <2% rate.

**Failure mode.** [Player-input destabilization](../../architecture/narrator-agent/failure-modes.md).

## Proof 5 — Honcho directional representations produce the right epistemic asymmetry

**Measurement.** The narrative designer audits 5 randomly-selected beta workspaces. For each, they check: does Aghora know things Tonto doesn't and vice versa? Does the eastern hall stay a *Sans-Nom secret* in playthroughs where he didn't tell Aghora about it? Pass: all 5 workspaces show clean asymmetry; no character knows what they shouldn't.

**Failure mode.** [Pillar 2](../../principles/pillars/02-characters-witness-bound.md) breaking. Convenient revelations creeping in.

## Proof 6 — The cost model holds

**Measurement.** Average per-turn cost across the beta cohort is within 20% of the projected ~$0.07/turn. See [per-turn cost model](../../project/cost/per-turn-model.md). Total beta-window spend is within budget (see [per-phase burn](../../project/cost/per-phase-burn.md)).

**Failure mode.** Cost runaway. If real cost is materially higher than projected, monetization math doesn't pencil and Phase 3 has a problem.

## Proof 7 — Players want to play it again

**Measurement.** Pass on *either* of the following:
- ≥40% of beta players who completed at least one playthrough start a second one within 2 weeks of their first; OR
- ≥65% of beta players respond positively to a post-playthrough survey question: "Would you start a new Sans-Nom if Phase 2 were available right now?"

The second measurement is the primary signal for Phase 1 — the slice may be too short to motivate a full second run unprompted, but *intent* is the underlying value being tested. Phase 1's childhood-only scope means some players who would eagerly replay a fuller game will reasonably wait. The survey catches this. If both measurements fail, that is a genuine signal that replay desire is broken. If one passes and one fails, use judgment.

**Failure mode.** No replay desire = [Pillar 4](../../principles/pillars/04-replay-is-medium.md) collapse. The medium is replay; if no one replays the medium isn't working.

## What it means to fail one of the seven

Phase 2 does not open. The team identifies which Phase-1 work-stream owns the failed proof, scopes a remediation, and re-runs the proof at the next beta-cohort review (~monthly). If the team accumulates 3+ failed proofs that don't respond to remediation, escalate to a project-level decision (re-scope, change architecture, or pause the project for re-evaluation).

## What it does NOT mean

- "Players love every scene." Some scenes will be weak. Phase 1 is the slice; Phase 2 is where breadth comes in.
- "The slice is feature-complete." Phase 1 is the *minimum* slice that proves the architecture; later phases build on it.
- "We've validated monetization." That's Phase 3's question. Phase 1 just needs the cost model to hold.
