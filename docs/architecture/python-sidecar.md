# Python Sidecar

A small FastAPI service that wraps the Honcho Python SDK and exposes it to the Go backend over local HTTP. The whole service is ~200 lines of FastAPI + Pydantic.

The reason it exists: there is no Go SDK for Honcho; the official Python SDK is first-class. Rather than reimplement Honcho's client behavior in Go, run a tiny Python service alongside the Go server. See [stack.md](stack.md) and [ADR 0001](decisions/0001-go-vs-typescript.md).

## Endpoints (six)

```
POST   /v1/scene/context         → fetch context bundle for Narrator (representations + recent scenes)
POST   /v1/scene/commit          → write scene messages, trigger directional updates
GET    /v1/peer/{id}/card        → fetch peer card (with optional ?target=)
PATCH  /v1/peer/{id}/card        → append/replace card facts (with optional ?target=)
POST   /v1/repr                  → directional representation query (observer + observed + search)
GET    /v1/health                → liveness
```

## Networking

- Local-only by default. Binds `127.0.0.1`.
- Auth: shared secret in env var (`SIDECAR_TOKEN`). Not internet-facing.
- Optional: Unix socket if you want to be paranoid about `localhost` exposure.

## Other responsibilities (Phase 2+)

- **Image generation** (Phase 2): proxy to Flux Pro via Replicate or Together. Image gen has better DX in Python than Go. Endpoints added under `/v2/images/*`.
- **TTS** (Phase 3): proxy to ElevenLabs. Endpoints added under `/v3/tts/*`.

These are added incrementally. Phase 1 ships with only the six Honcho endpoints + health.

## Why a sidecar instead of a separate service

- Single-process locality keeps latency low (sub-millisecond for a `/v1/scene/context` call's network hop).
- Single Docker image deployment — both Go and Python in one container.
- Shared lifecycle — if the Go server is up, the sidecar is up. No service discovery needed.

## Why not just call Honcho directly from Go via HTTP

- The Honcho Python SDK does meaningful client-side work (request shaping, retry, batching). Reimplementing it in Go is not worth the effort.
- Future-proofs against Honcho SDK evolution. When Plastic Labs ships SDK v2, the upgrade is a Python change.

## Schema management

Pydantic models on the Python side. Equivalent Go structs in `internal/bridge/`. Phase 0 deliverable: a generated-code path so the Go structs stay in sync with the Pydantic models. (Manual for v1 if generation tooling is fussy.)

## Trace propagation

Trace IDs propagate Go → Python sidecar → Honcho. OpenTelemetry is the path of least resistance. See [observability.md](observability.md).

## Failure modes

- **Sidecar dead.** Go server returns degraded-mode response to player ("the world dims for a moment...") and emits a critical telemetry event. Auto-restart via Docker.
- **Honcho dead.** Sidecar surfaces 503 to Go; Go falls back to in-memory scene state for the current turn (with logged warning). Resume on Honcho recovery.
- **Sidecar slow.** Per-call latency instrumented. P99 > target triggers alert.
