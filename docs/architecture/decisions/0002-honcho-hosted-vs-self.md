# ADR 0002 — Honcho hosting: Hosted in Phase 1, self-host as Phase-3 escape hatch

## Status

**Accepted.** 2026-05-03.

## Context

Honcho can be used as a hosted service (Plastic Labs' managed offering) or self-hosted. The choice has consequences for cost, latency, operational complexity, and team focus.

Constraints:

- 4-person team. Operational scope must stay narrow.
- Phase 1 beta cohort is small (~50 testers). Hosted handles this without question.
- Phase 3 production cost target depends on monetization model. BYOK and subscription tiers absorb cost differently.
- Self-hosting Honcho would let the team tune reasoning pipeline costs but adds DBA-style work.

## Decision

**Phase 1: hosted.** Phase 2: hosted. **Phase 3: re-evaluate; keep self-host as documented escape hatch.**

The decision criterion at the Phase 3 re-evaluation: if Honcho hosted cost-per-playthrough exceeds the per-playthrough monetization yield by a margin that BYOK alone cannot close, switch to self-hosted.

## Consequences

### Positive (hosted in Phase 1+2)

- Zero infra effort. Team focuses on the agent loop.
- Cross-device save/resume works naturally via account-bound workspaces (verifies a Phase-1 deliverable for free).
- Plastic Labs handles upgrades, scaling, backups.

### Negative (hosted)

- Cost is set by Plastic Labs' pricing, not by the team.
- Latency is set by Plastic Labs' infra, not by the team. (If their region is far from the game's, every turn pays the network cost.)
- Outage handling is dependent on Plastic Labs' SLA.

### Phase 3 escape hatch

If the Phase-3 re-evaluation triggers self-host:

- Containerize Honcho on the same Docker host as the Go + Python sidecar.
- Provision a managed Postgres (Honcho's primary store).
- Migrate workspaces via Honcho's export/import tooling (verify this exists in Phase 0 — it's an open question).
- Bring monitoring up to parity with hosted.

This work is estimated at 2 engineer-weeks. Time-bounded.

## Re-evaluation

Re-open this ADR at Phase 3 production planning, or earlier if:

- Hosted Honcho has an outage longer than the team's tolerance for player-facing downtime.
- Hosted pricing changes materially.
- Plastic Labs ships a feature only available self-hosted that the project needs.
