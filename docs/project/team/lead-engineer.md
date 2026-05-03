# Lead Engineer

## Owns

- The agent loop end-to-end. See [authoring loop](../../architecture/narrator-agent/authoring-loop.md).
- Honcho integration. Workspace bootstrap, session boundary policy, representation queries, dreaming configuration. See [`architecture/honcho-integration/`](../../architecture/honcho-integration/).
- The Python sidecar (small surface; ~6 endpoints + Phase 2/3 additions). See [python-sidecar.md](../../architecture/python-sidecar.md).
- Lore retrieval pipeline. Vector store, embedding strategy, per-turn search. See [lore-retrieval.md](../../architecture/lore-retrieval.md).
- Deterministic scaffolding for canonical events (the few hard-coded things, like the Bushitaka decision arc gates).
- Repo structure, build, deploy. See [repo-structure.md](../../architecture/repo-structure.md).
- ADR maintenance. New architecture decisions get ADRs.

## Collaborates with

- **Narrative designer** on prompt-template changes that span code + content boundaries (e.g., the [interpretation prompt](../../mechanics/input/interpretation-prompt.md) lives in `lore/` but its assembly is in `internal/prompt/`).
- **Frontend engineer** on the WebSocket / SSE protocol for streaming Narrator output.
- **Producer** on observability dashboards and cost tracking.

## Phase responsibilities

- **Phase 0** — Honcho integration end-to-end, agent loop scaffolding, Python sidecar, lore retrieval pipeline, CLI testbed.
- **Phase 1** — production-hardening the loop, WebSocket streaming, telemetry pipeline, account-bound workspace lifecycle.
- **Phase 2** — multi-session integration testing, image gen pipeline (sidecar additions), death-state UX integration.
- **Phase 3** — chest-crest mechanic (architectural exception to witness-bound), TTS pipeline, monetization flow.

## Hiring profile

- Go fluent. Familiar with goroutines, contexts, structured concurrency.
- Comfortable with Python (sidecar work) but doesn't need Python depth.
- Has worked with LLM APIs in production (rate limiting, retries, streaming, cost tracking).
- Cares about observability. Will instrument from day one without prompting.

## Anti-pattern

A pure-Go engineer who insists on rewriting the Honcho SDK in Go. Adds maintenance burden for no real gain. See [ADR 0001](../../architecture/decisions/0001-go-vs-typescript.md).
