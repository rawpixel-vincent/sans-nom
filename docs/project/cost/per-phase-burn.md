# Per-Phase Burn Rate

## Phase 0 — Pre-production

4 devs hand-playtesting the agent loop:

- ~50 turns/dev/day × 4 devs × $0.07/turn = **$14/day**
- Across 1 month (~22 working days): **~$300 total**

Negligible against unlimited-AI access budget.

## Phase 1 — Vertical slice (closed beta)

Closed cohort of ~50 testers:

- Each runs ~2 playthroughs of ~4 hours each = ~1,440 turns per tester
- 50 testers × 1,440 turns × $0.07 = **~$5,000 across the beta window**

Real money. Bound it. Use cohort sign-up + per-tester turn cap to prevent runaway.

## Phase 2 — Adolescence (broader beta)

Cohort grows to ~500 testers + image generation:

- Per-tester turn count similar (multi-session continuity adds some, image-gen costs add some)
- 500 testers × ~1,800 turns × $0.07 = **~$63,000**
- Image gen at ~30% opt-in adoption × ~50 images per playthrough × $0.03/image × 500 testers = **~$22,500**
- **Total Phase 2 burn: ~$85,000**

This is meaningful. Mitigations:

- Per-tester turn cap stays in place.
- Image gen is opt-in (the toggle is the cost lever).
- BYOK option offered to testers willing to provide their own keys (reduces studio cost for that subset).

## Phase 3 — Production v1.0

Depends entirely on monetization model:

- **BYOK:** studio carries near-zero LLM cost; players pay their own. Studio carries Honcho hosting + infrastructure.
- **Subscription:** studio carries LLM cost up to subscription cap. At $15/month, cap might be ~200 turns/month per subscriber. Above-cap usage either degrades, queues, or upcharges.
- **Single-purchase + capped runs:** studio fronts cost; player pays once. At $30 for ~5 playthroughs of credits, studio's cost-of-goods is ~$50 of LLM, so this model only works with margin from non-playing buyers + bulk discounts.

See [monetization options](monetization-options.md) for the trade-offs.

## Cost discipline mechanisms (always on)

- Per-turn cost telemetry. Threshold alert if avg per-turn cost moves above $0.10 over a rolling 24h window.
- Per-tester turn cap in beta phases.
- Model-tier discipline. Opus only for Narrator; Sonnet for actors and audit-pass; Haiku for cheap utility.
- Caching where possible (lore retrieval results cached per session; static peer cards cached per workspace).
- Streaming with abort cost amortization (mid-stream abort still pays full token cost; UX should make abort rare).

## Re-evaluation

Re-cost at each phase boundary. Anthropic pricing changes, real per-turn cost data from beta, and operational cost-of-living should all feed back into next-phase projections.
