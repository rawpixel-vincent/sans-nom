# Observability

The agent loop has more failure modes than a normal product, and most of them are silent (Narrator drift, actor-bleed, lore-retrieval misses). Instrument these from the first commit.

## Metrics (must instrument before Phase 1)

- **Per-turn token usage** by agent (Narrator vs. each actor). Bucketed by tier (Opus, Sonnet).
- **Per-turn latency** broken into Narrator / actor-fanout / Honcho / lore-retrieval segments.
- **Lore-retrieval hit/miss/score distribution.** A cold hit rate above 30% means the corpus or the embedding strategy needs work.
- **Scene-length distribution.** Narrator drift toward generic sci-fi often shows up as scenes getting longer or shorter than the established baseline.
- **Card-update events.** Every `set_card` call logged with diff. Lets you replay the dynasty's evolution.
- **Player free-text vs. choice-pick ratio.** Tells you whether the choice-list is doing its job or being ignored.

## Metrics (add in Phase 1 if not already)

- **Generated-NPC count per session.** Too many dilutes IP flavor. See [generated-npcs](../mechanics/cast/generated-npcs.md).
- **Actor-call-count per turn distribution.** Catches scenes that fan out beyond the cap.
- **Audit-pass drift-flag rate.** Climbing rate is a leading indicator.
- **Card cap hit rate.** Catches when [eviction policy](../mechanics/memory-model/card-eviction.md) is biting earlier than expected.

## Traces

- Trace IDs propagate Go → Python sidecar → Honcho. OpenTelemetry is the path of least resistance.
- Each turn produces one root trace with spans for: context-assembly, lore-retrieval, actor-fanout (parent + per-actor child spans), narrator-compose, scene-commit.
- Streaming: the streaming span covers the streamed prose only; the commit span runs after stream completion.

## Logging

- Structured JSON. Required fields per turn: `turn_id`, `workspace_id`, `session_id`, `player_input` (or choice ID), `narrator_output_length`, `actors_invoked`, `tokens_total`, `latency_ms_total`.
- Sensitive content (player input, generated prose) logged at debug level only. In prod, redacted by default. PII boundaries respected — players' free-text is the player's text, not training data.

## Alerting (Phase 1+)

- Critical: sidecar down, Honcho 5xx rate > 5%, Anthropic API outages.
- Warning: P99 turn latency > target, drift-flag rate climbing, card-update apply rate falling.
- Informational: cohort-level metrics for beta playtest analysis.

## Quality auto-grader

A separate cheap-model agent (Sonnet) runs on a sample of in-prod scenes against the [regression corpus](../phases/phase-0-preproduction/deliverables.md). Computes a quality score (voice fit, canonical fidelity, scene cohesion). Score distribution shifts trigger an alert. See [Failure 7 in failure-modes](narrator-agent/failure-modes.md).

This is the closest thing to a "tests passing" signal the agent loop can have. It is not a CI gate (too expensive to run per PR) — it's a continuous health check.

## What this rules out

- "We'll add observability later." Telemetry is not a polish layer; it's the only way to detect silent failures. Ship it from commit one.
- Anonymous scene replay for debugging. PII boundaries.
- Storing player input long-term beyond what Honcho already retains. Workspace deletion deletes Honcho state; logs roll over per retention policy.
