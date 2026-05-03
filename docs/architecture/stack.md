# Stack

Resolved by [ADR 0001](decisions/0001-go-vs-typescript.md). The original GDD draft contained a contradiction (Section 4 said "Python or TypeScript, lead engineer's choice"; Appendix C committed to Go + Python sidecar). This file is the single source of truth.

| Layer | Choice | Rationale |
|---|---|---|
| Backend language | **Go 1.22+** | Reuses Council of Castaka patterns (prompt assembly, TUI compositor). Goroutines fit the actor-fanout model cleanly. Single-binary deployment. |
| Memory service | **Honcho via Python sidecar** | Official Python SDK is first-class; no Go SDK exists. Run as `uv`-managed FastAPI service; Go talks to it over local HTTP. See [python-sidecar.md](python-sidecar.md). |
| LLM provider | **Anthropic** primary | Opus for Narrator, Sonnet for actors. Single-vendor billing/observability. OpenRouter as fallback for outage. |
| Embeddings / lore retrieval | Anthropic + a vector store (probably **Qdrant** or **pgvector**) | Lore corpus is finite and pre-embedded once. No live indexing needed. See [lore-retrieval.md](lore-retrieval.md). |
| CLI testbed (Phase 0) | **Bubble Tea** (`charmbracelet/bubbletea`) | Council of Castaka already uses it. Region-based compositor in `internal/ui/render/render.go` is reusable. |
| Web frontend (Phase 1) | **React or Svelte** + Tailwind | Designer's preference. Talks to Go backend over WebSocket for streaming Narrator output. See [ADR 0003](decisions/0003-streaming-narrator-output.md). |
| Image gen (Phase 2) | **Flux Pro via Replicate** or **Together** | Routed through the Python sidecar — image gen has better DX in Python than Go. |
| TTS (Phase 3) | **ElevenLabs** | Routed through the Python sidecar for the same reason. |
| Deployment | Single Docker image containing Go binary + Python sidecar; Honcho hosted | Simpler than splitting infra. Honcho self-host is a Phase 3 escape hatch if cost forces it — see [ADR 0002](decisions/0002-honcho-hosted-vs-self.md). |

## What this rules out

- A pure-Python or pure-TS implementation. Either would forfeit Council of Castaka leverage. ADR 0001 walks through the trade.
- Multi-vendor LLM routing as primary architecture. Adds operational complexity without near-term benefit. OpenRouter is fallback only.
- Self-hosted Honcho in Phase 1. Re-evaluated when cost or latency forces it; until then, hosted.
