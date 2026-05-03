# Producer / Playtest Lead

## Owns

- Beta cohort recruitment, communication, and management.
- Telemetry dashboards and observability data interpretation. See [observability.md](../../architecture/observability.md).
- Cost monitoring. Per-tester turn cap enforcement, per-phase burn tracking. See [`cost/`](../cost/).
- Monetization decision-making. Which model, when to commit, how to roll out. See [monetization-options.md](../cost/monetization-options.md).
- Marketing rhythm. Devlogs, beta sign-up funnel, launch communications.
- Risk register maintenance. See [`risks/`](../risks/).
- Decisions log (cross-cutting, non-architectural). See [decisions-log.md](../decisions-log.md).
- (If technically inclined) Owns the dreaming/peer-card-update rules and the scene-quality auto-grader. This is partial overlap with the lead engineer; the boundary is determined by who's strongest.

## Collaborates with

- **Lead engineer** on observability instrumentation, cost data, telemetry alerts.
- **Narrative designer** on beta playthrough scene reviews, regression corpus prioritization, IP-violation audits.
- **Frontend engineer** on beta-cohort onboarding flow, telemetry events from the frontend.

## Phase responsibilities

- **Phase 0** — observability dashboards, cost monitoring, schedules the Phase-0 → Phase-1 review.
- **Phase 1** — closed beta cohort recruitment, weekly playtest reviews, devlog publishing rhythm, cost discipline.
- **Phase 2** — beta cohort growth (50 → 500), new dashboard metrics (image opt-in, multi-session continuity errors, NPC count distribution).
- **Phase 3** — v1.0 launch readiness, beta-to-prod cohort migration, marketing campaign, monetization model selection and rollout.

## Hiring profile

- Has shipped at least one production indie game. Knows what a beta cohort feels like at 50 testers vs. 500.
- Comfortable reading dashboards, surfacing trends, reasoning about cost models.
- Comfortable with judgment calls under uncertainty. The ADR / decisions log machinery exists *because* this role makes decisions; the documentation is the artifact, the call is the work.
- Cares about the IP. The team is small; surface-level engagement is not enough.

## Anti-pattern

A producer who treats the project as feature-list management. The agent loop and the Narrator's voice are not features; they are the work. The producer's job is to make space for that work to land, not to schedule it as tasks.
