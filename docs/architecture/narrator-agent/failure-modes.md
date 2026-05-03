# Narrator Failure Modes

The agent loop has more failure modes than a normal product, and most of them are silent. This file enumerates them and the concrete defenses each one warrants.

## Failure 1 — Drift

**What it is.** After 200 scenes the Narrator forgets it's writing Metabarons and starts writing generic sci-fi.

**Defense.**

- Every Narrator call retrieves canon-grounded context — see [lore-retrieval.md](../lore-retrieval.md).
- The long-running Narrator [peer card](peer-card.md) is re-asserted in every turn's context.
- **Audit pass.** A separate cheap-model agent (Sonnet) reads the last 20 scenes once per session and flags drift. Trigger cadence: end-of-session (cheaper than every-N-scenes; matches the natural pause for the [scene-end checkpoint](../../mechanics/memory-model/card-updates.md)). Acceptance criteria: scenes are evaluated against the [Phase 0 regression scene corpus](../../phases/phase-0-preproduction/deliverables.md) — specifically, the audit pass uses 5 randomly sampled corpus scenes as voice-reference and compares against the last 20 played scenes.
- **Failure action.** If drift is flagged, the audit pass surfaces the specific drifted scenes plus the most-similar canonical references and re-injects those references into the next 5 turns' Narrator context. If drift persists past 5 turns, flag for human review (in dev) or emit a degraded-mode telemetry event (in prod) — the game continues but quality is now an explicit concern.

The original GDD's defense was named as "audit pass + dreaming" without specification. This file makes the trigger, criteria, and failure action concrete.

## Failure 2 — Actor-bleed

**What it is.** An actor agent generates dialogue that sounds out-of-character because its peer card got polluted by misinterpreted Narrator scene-setup.

**Defense.**

- Actor peer cards are *immutable seed* + Honcho-managed evolution. The seed never gets edited. See [card-seeding.md](../actor-agents/card-seeding.md).
- Evolution happens only through Honcho's pipeline (dreaming + Narrator-triggered updates), which is auditable.
- Per-turn, the actor agent receives only its scoped representation and the Narrator's scene-setup prompt — not the previous scene's prose. This breaks the contamination chain.

## Failure 3 — Player-input destabilization

**What it is.** Free-text input that breaks the world ("I summon a dragon and become emperor"). Or fourth-wall breaks ("you are an AI"). Or canonical-character commandeering ("Aghora says: <player-typed dialogue>").

**Defense.**

- The Narrator's [interpretation prompt](../../mechanics/input/interpretation-prompt.md) instructs it to write what *would happen* (nothing; or characters react with confusion or contempt). Not a refusal screen; a fictional response. See [edge cases](../../mechanics/input/edge-cases.md) for the full taxonomy.
- A thin **rule-based pre-filter** sits in front of the Narrator. Catches: literal "ignore previous instructions," role-prefix attempts ("Aghora:" prefix in player input), and abuse (slurs, CSAM, personal attacks). Filter is a Go-side string/regex check, not an LLM call. See [Phase 0 deliverables](../../phases/phase-0-preproduction/deliverables.md) for the spec.

## Failure 4 — Cost runaway

**What it is.** Each turn might involve 1 Narrator call + N actor calls + Honcho updates. With N actors growing, costs scale badly.

**Defense.**

- Model-tier discipline: Opus only for Narrator; Sonnet/Haiku for actors and audit-pass.
- Cap on simultaneously-queried actors per turn. Default cap: 4. Beyond that, the Narrator must consolidate (treat ambient cast as background, query only foregrounded actors). See [observability](../observability.md) for the metric.
- BYOK monetization is the user-facing safety net. See [monetization options](../../project/cost/monetization-options.md).

## Failure 5 — Honcho session boundaries

**What it is.** Trying to use one infinite session loses Honcho's reasoning quality at scale.

**Defense.** Explicit per-arc session design. See [`honcho-integration/sessions.md`](../honcho-integration/sessions.md).

## Failure 6 — Narrator boring after 30 hours

**What it is.** Long single-player playthroughs reveal Narrator repetition (same metaphors, same scene shapes, same beats).

**Defense.**

- Audit pass (Failure 1) also flags repetition signals: scene-length distribution flattening, recurring phrase detection.
- Beta long-play testing in Phase 1 surfaces this before Phase 2 commits.
- Acceptable failure mode if rare. If common, escalate to peer-card revision or fine-tuning consideration.

## Failure 7 — Scene quality degrades when load shifts

**What it is.** Cost-saving moves (Sonnet for actors, batch dreaming) cause subtle quality drops the team didn't catch in tier-by-tier testing.

**Defense.**

- Per-turn token usage and per-tier latency instrumented from day one. See [observability.md](../observability.md).
- Quality auto-grader runs on a sample of in-prod scenes against the regression corpus. Score distribution shift triggers an alert.

## What's *not* a defense

- "Just rerun the scene." Reruns waste tokens and still don't address the underlying issue.
- "Fall back to a hand-written scene." There are no hand-written scenes in this design beyond the regression corpus. Fallback would break the architecture's premise.
- "Restrict the player from doing things." [Pillar 3](../../principles/pillars/03-narrator-as-author.md) forbids this. Defenses operate on the Narrator side, not the player side.
