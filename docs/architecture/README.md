# Architecture

The HOW of sans-nom. Engineer's reference for the agent loop, the memory substrate, the stack, and the decisions that bind them.

For game-design context (what the agent loop is *for*), see [`../mechanics/`](../mechanics/README.md). For phase-bound build scope, see [`../phases/`](../phases/README.md).

## Contents

- [`stack.md`](stack.md) — language, frameworks, providers (resolved by [ADR 0001](decisions/0001-go-vs-typescript.md))
- [`agent-topology.md`](agent-topology.md) — Player → Narrator → Actors → Honcho diagram and roles
- [`narrator-agent/`](narrator-agent/) — peer card, authoring loop, failure modes, dev-loop ergonomics
- [`actor-agents/`](actor-agents/) — invocation contract, card seeding
- [`honcho-integration/`](honcho-integration/) — peers, sessions, representations, dreaming
- [`lore-retrieval.md`](lore-retrieval.md) — corpus ingestion plan + retrieval at turn time
- [`repo-structure.md`](repo-structure.md) — monorepo layout
- [`python-sidecar.md`](python-sidecar.md) — six-endpoint FastAPI service wrapping the Honcho SDK
- [`observability.md`](observability.md) — metrics + traces from day one
- [`decisions/`](decisions/) — Architecture Decision Records (ADRs), append-only
  - [`0001-go-vs-typescript.md`](decisions/0001-go-vs-typescript.md)
  - [`0002-honcho-hosted-vs-self.md`](decisions/0002-honcho-hosted-vs-self.md)
  - [`0003-streaming-narrator-output.md`](decisions/0003-streaming-narrator-output.md)
  - [`0004-card-eviction-policy.md`](decisions/0004-card-eviction-policy.md)

## ADR conventions

Each ADR is a short markdown file with these sections: **Status** (proposed | accepted | superseded), **Context**, **Decision**, **Consequences**. ADRs are append-only — superseding an ADR means writing a new one and flipping the old one's status to `superseded by 000N`.
