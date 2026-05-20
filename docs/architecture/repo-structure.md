# Repo Structure

The repo is a Go-first monorepo with a small Python sidecar and a Svelte admin
UI. The current implementation is centered on lorepack-authored scenario
contracts, bounded story packets, scoped model-visible workspaces, and Honcho
scene memory.

```text
sans-nom/
|-- admin/                 # Svelte admin UI, including Behind The Scenes
|-- bridge/                # Python FastAPI sidecar for Honcho, lore index, TTS
|-- cmd/
|   |-- cli/               # Interactive CLI and full-screen TUI play mode
|   |-- prompt-eval/       # Prompt/provider diagnostic runner
|   |-- codex-*            # Codex probe utilities
|   `-- provider-*         # Provider prompt probes
|-- data/
|   |-- seed-runs/         # Generated seed artifacts and agent workspaces
|   `-- stories/           # Story state, packets, turns, audio, agent workspaces
|-- docs/                  # Project documentation
|-- internal/
|   |-- actors/            # Actor registry and invocation support
|   |-- admin/             # Admin API, logs, backstage transcript mapping
|   |-- agentfs/           # Curated per-run model-visible workspaces
|   |-- bridge/            # Go HTTP client for the Python sidecar
|   |-- clitui/            # Bubble Tea launcher and play UI
|   |-- codexoauth/        # Codex-backed model runner and scoped local tools
|   |-- gamelog/           # JSONL/detail logging and redaction
|   |-- lore/              # Lore search/client plumbing
|   |-- loreexpert/        # Lore authority/reference tool
|   |-- lorepack/          # Pack loading, validation, scenario contract
|   |-- modelmeta/         # Provider/model/reasoning metadata extraction
|   |-- narrator/          # Narrator rollout
|   |-- prompt/            # Prompt assembly for Writer/Narrator
|   |-- scenarist/         # Sequential Scenarist tool/session support
|   |-- scene/             # Story packet/node schemas and decisions
|   |-- seed/              # Seed generation and verification
|   |-- story/             # Story workspace persistence
|   |-- storyapp/          # Shared play operations
|   |-- telemetry/         # Runtime events
|   |-- tts/               # Voice synthesis
|   `-- writer/            # Writer refresh runners
`-- lore/
    `-- packs/
        `-- metabarons/
            `-- lore-pack.json
```

## Runtime Data

`data/` is runtime output, not source design material:

- `data/seed-runs/<seed_id>/agent/` is the scoped workspace visible to seed
  Writer and Scenarist tools.
- `data/stories/<story_id>/agent/<role>/<run_id-or-turn>/` is the scoped
  workspace visible to runtime Writer/Narrator tools.
- `data/stories/<story_id>/packets/` stores accepted packets.
- `data/stories/<story_id>/turns/` stores player-facing narrated turns.
- `data/stories/<story_id>/audio/` stores generated TTS output.

The application can read old story and seed data for resume/debugging, but model
tools should only see staged files inside the current agent workspace.

## Lorepacks

`lore/packs/<pack_id>/lore-pack.json` is the source of truth for pack metadata
and the authored gameplay contract. In Faithful Canon Prototype mode, the pack
contract defines the story premise, protagonist role, opening situation,
must-honor constraints, must-not-violate constraints, and choice principles.

Model-visible tools do not read raw `lore/packs/...` paths. The active pack is
copied into the current agent workspace as `lorepack/`.

## Go And Python Boundary

- Go owns CLI/TUI, story persistence, prompt assembly, model orchestration, and
  admin APIs.
- The Python sidecar wraps Honcho SDK operations and auxiliary services that are
  Python-native.
- Communication is local HTTP over `127.0.0.1`.

## What Is Not In The Repo

- Raw comic page scans. They belong under ignored external asset storage.
- Honcho hosted state. It lives in Honcho and is addressed by story/session ids.
- Model provider secrets. Local env/config supplies them.

## Bootstrap Shape

A local dev loop typically:

1. Starts the bridge sidecar.
2. Runs `go run ./cmd/cli` for interactive play or `go run ./cmd/cli -admin`
   for the admin server.
3. Selects a lorepack.
4. Validates the lorepack scenario contract.
5. Creates a seed or resumes a story.
6. Stages scoped agent workspaces as model calls happen.

See [agent-filesystem](agent-filesystem.md), [writer-narrator-split](writer-narrator-split.md),
and [python-sidecar](python-sidecar.md).
