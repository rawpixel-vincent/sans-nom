# ADR 0003 — Stream Narrator output to UI; commit to Honcho post-stream

## Status

**Accepted.** 2026-05-03.

## Context

The Narrator's prose is the player's main experience. Latency between input and first token of output dominates perceived responsiveness. Two modes:

- **Blocking.** Narrator generates fully; commit to Honcho; return to UI. Player waits the full generation time before seeing any text.
- **Streaming.** Narrator tokens stream to UI as generated. Honcho commit happens after stream completion, with the full prose.

Streaming feels much better. Complication: the commit-to-Honcho step needs the *complete* prose, not the tokens. This means:

- Final prose is what gets committed (matches the player's view of canon).
- The streamed tokens are *display only* — they are not stored anywhere except as they accumulate into the final prose buffer.
- Mid-stream cancellation (player aborts) needs handling: do we commit the partial stream, or roll back?

## Decision

**Stream prose to UI; commit committed-text to Honcho post-stream.**

Mid-stream cancellation rolls back: nothing committed to Honcho, no card updates, no state change. The aborted scene is gone. (Players can re-enter the same input if they want.)

## Consequences

### Positive

- Perceived latency drops from ~5s (full Opus generation) to ~500ms (first token).
- Honcho commit semantics stay clean: only complete scenes get committed.
- Aborted scenes don't pollute Honcho with half-resolved state.

### Negative

- Streaming requires WebSocket (or SSE) on the frontend. Adds a small amount of frontend complexity.
- Token-level cost is fully incurred even on aborted streams. Acceptable — abort should be rare.
- Determinism is harder. Streaming output is not byte-identical to a same-prompt blocking call (sampling is per-token; minor differences possible). For Phase 0 regression testing, the corpus uses blocking mode to keep diffs clean.

### Replay determinism

The original draft asked (Appendix C.8 #2) whether playthroughs should be deterministic given the same RNG seed + Honcho state. **Decision: no, normal play is non-deterministic** (LLM sampling). For QA there is a `replay-deterministic` flag on the CLI testbed that uses temperature=0 and a fixed seed where the LLM allows; this is a debugging tool, not a player feature.

## Re-evaluation

- If frontend WebSocket complexity proves too costly for Phase 1, fall back to SSE (Server-Sent Events). Same UX, simpler protocol.
- If a player feature (replay-as-time-travel, deterministic shareable runs) wants determinism, design the feature explicitly; don't pull it from this ADR.
