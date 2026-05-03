# Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Narrator voice doesn't hold; players smell "AI slop" | Medium | Existential | Phase 0 voice lock with extensive playtest before any other work. Designer's full attention for the first month. See [Phase 0 exit criteria](../../phases/phase-0-preproduction/exit-criteria.md). |
| Honcho's directional representations are slower than expected at story-length context | Medium | High | Talk to Plastic Labs early. Profile in Phase 1. Self-host Honcho if perf forces it ([ADR 0002](../../architecture/decisions/0002-honcho-hosted-vs-self.md)). |
| Cost-per-playthrough makes monetization impossible | Medium | High | BYOK is the safety net. Architecture supports it from day one. See [monetization options](../cost/monetization-options.md). |
| Players free-text-attack the world in ways the Narrator can't author through gracefully | Low | Medium | Authoring policy + thin abuse filter. Test in Phase 0 corpus + Phase 1 beta. Iterate on the Narrator card. See [edge cases](../../mechanics/input/edge-cases.md). |
| The IP licensing question (Humanoids/Jodorowsky) | High | Critical | See [license.md](license.md) — three tracks defined. Decision deadline: end of Phase 1. |
| Player base is too small (niche Metabarons fans + AI-curious overlap) | Medium | Medium | Position as a *general* AI-narrative game with the Metabarons as its first IP, with the technology being the through-line. Future titles in other IPs reuse 80% of the engineering. See [why-this-team](../../principles/why-this-team.md). |
| 4-person team burns out in the prompt-tuning loop | Medium | High | Discipline: time-box prompt iteration to working hours; set a quality bar (regression corpus passes) and stop fiddling beyond it; ship beta early to get external signal. |
| The Narrator gets boring after 30 hours of one player's playthrough | Medium | Medium | [Audit pass](../../architecture/narrator-agent/failure-modes.md) and dreaming. Active long-play testing during beta. Acceptable failure mode if rare. |
| Beta playthroughs produce bizarre out-of-canon storylines that get screencapped and damage IP reputation | Low | Medium | Beta is opt-in with a content notice. Public-facing trailer scenes are curated. Not a product-killer; a marketing shape. |
| Honcho hosted has an outage during a critical beta window | Low | Medium | Sidecar surfaces 503; Go falls back to in-memory degraded mode for current turn. Resume on recovery. See [python-sidecar.md](../../architecture/python-sidecar.md). |
| Image generation (Phase 2) produces consistently off-style results | Medium | Medium | Style-lock via prompt prefix; style reference set curated by designer. If quality unfixable, ship without (image gen is opt-in, not required). |
| TTS (Phase 3) costs balloon | Medium | Medium | Per-character voice caching where possible. Opt-in toggle is the safety net. Cut TTS from Phase 3 if it threatens v1.0 launch. |
| Card eviction loses load-bearing facts | Low | High | Narrator-curated policy + reference-timestamp tracking ([ADR 0004](../../architecture/decisions/0004-card-eviction-policy.md)). Eviction events logged for replay. |

## Maintenance

This register is reviewed at each phase boundary. New risks identified during playtest go here; mitigated risks get marked accordingly. The register is a living document — risks that *don't* materialize over a phase get downgraded, not deleted (so we remember what we feared).
