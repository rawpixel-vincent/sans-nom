# ADR 0001 — Backend language: Go primary, Python sidecar

## Status

**Accepted.** 2026-05-03.

## Context

The original GDD draft contained a contradiction: Section 4 said *"Python or TypeScript, lead engineer's choice"*; Appendix C.1 committed to *"Go 1.22+ … Python sidecar."* Same document, two stacks. This ADR resolves the contradiction.

Constraints:

- Honcho's official SDK is Python. There is no Go SDK.
- The Council of Castaka (the user's existing Metabarons agent-orchestration system, at `external/references/council of castaka/`) is written in Go and uses Bubble Tea for its TUI. Its prompt-assembly patterns and UI region-compositor are directly reusable.
- The team is 4 people. Discrete pieces of architecture should fit one engineer's mental model end-to-end.

Options considered:

- **A. Pure Python** — single language, native Honcho SDK. Forfeits Council of Castaka leverage. Async-Python concurrency is workable but worse than goroutines for the actor-fanout shape.
- **B. Pure TypeScript** — modern ergonomics. No Honcho SDK; would need to wrap the Honcho REST API ourselves or call Python via subprocess. No Council of Castaka leverage. Doubles the surface area of new code.
- **C. Go primary + Python sidecar** — Council of Castaka leverage retained. Honcho native via Python. Two-language repo, but the Python surface is bounded (see [python-sidecar.md](../python-sidecar.md)).
- **D. Go primary, Honcho via REST in Go** — single language, but reimplements Honcho client behavior. Maintenance burden on Honcho SDK upgrades.

## Decision

**Choose C — Go primary + Python sidecar.**

The Council of Castaka leverage is the single largest engineering accelerator the project has. Forfeiting it costs more than the two-language repo costs. The Python sidecar is bounded to ~6 endpoints (~200 LOC of FastAPI + Pydantic) and does not grow unbounded.

The Section 4 hedge ("Python or TypeScript") is deleted; [`stack.md`](../stack.md) is now the single source of truth.

## Consequences

### Positive

- Council of Castaka prompt-assembly and UI patterns reused verbatim in Phase 0 (CLI testbed) and Phase 1 (server-side prompt assembly).
- Goroutines fit the actor-fanout pattern cleanly.
- Single-binary deployment (Go) + single-image Docker (Go + Python).
- Honcho SDK upgrades are Python changes only; the Go side is insulated.

### Negative

- Two languages in one repo. Engineers need basic Python literacy.
- Pydantic ↔ Go struct synchronization is a maintenance task. Phase 0 deliverable: a generated-code path or a manual sync convention.
- Sidecar is one more thing that can fail. Mitigated by single-process locality (no service discovery; if the Go server is up, the sidecar is up).

### Re-evaluation

Re-open this ADR if:

- Honcho releases an official Go SDK (Plastic Labs has not signaled this).
- The Python sidecar grows beyond ~500 LOC of business logic (it should not).
- Operational pain from the two-language repo materially exceeds the Council of Castaka leverage savings.
