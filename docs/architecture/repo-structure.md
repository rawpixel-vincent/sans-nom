# Repo Structure

The Go monorepo layout. Adapted from Council of Castaka patterns.

```
sans-nom/
├── cmd/
│   ├── cli/              # Phase 0 Bubble Tea testbed
│   └── server/           # Phase 1+ web backend
├── internal/
│   ├── narrator/         # Narrator agent: peer card, scene-build loop
│   ├── actors/           # Actor agent registry, per-character invocation
│   ├── scene/            # Scene state, choice generation, free-text resolution
│   ├── lore/             # Canon retrieval — vector search over character cards + comic transcripts
│   ├── bridge/           # HTTP client for the Python sidecar (Honcho + image gen + TTS)
│   ├── prompt/           # Prompt assembly — adapted from council-of-castaka/internal/council/prompt.go
│   └── ui/               # Bubble Tea components — adapted from council-of-castaka/internal/ui
├── bridge/               # Python sidecar (uv-managed)
│   ├── pyproject.toml
│   ├── src/bridge/
│   │   ├── main.py       # FastAPI app
│   │   ├── honcho_ops.py # SDK wrapper exposing the 6 endpoints
│   │   ├── images.py     # Phase 2
│   │   └── tts.py        # Phase 3
│   └── tests/
├── lore/                 # Source of truth — copied/symlinked from council-of-castaka
│   ├── narrator/
│   │   └── peer-card.md  # The Narrator's peer card; hot-reloaded in dev
│   ├── actors/           # Per-character cards (one per file)
│   ├── relations/        # The directional relationship matrix
│   └── canon/
│       └── comics/       # Comic page transcripts (extracted text + page refs)
├── scripts/
│   ├── seed-honcho.go    # One-shot: bootstrap workspace with seeded peer cards
│   └── eval-narrator.go  # Run the Narrator against the test-scene corpus
├── testdata/
│   └── scenes/           # Phase 0 hand-authored test scenes (the regression set)
├── docs/                 # This documentation tree
├── Makefile              # `make dev` boots both Go and Python
├── docker-compose.yml    # Local dev: Go + Python + (optional) Qdrant
└── README.md
```

## Why this layout

- `cmd/` separates the CLI testbed from the server. Both share `internal/`.
- `internal/` keeps Go packages private. Public API is via the server's HTTP/WebSocket interface, not Go imports.
- `bridge/` is a sibling Python project, not nested under `internal/`. Independent dependency management via `uv`.
- `lore/` is git-versioned content — narrative designer's working area. Hot-reloaded by the Go server in dev. See [dev-loop-ergonomics](narrator-agent/dev-loop-ergonomics.md).
- `testdata/scenes/` is the regression corpus. See [Phase 0 deliverables](../phases/phase-0-preproduction/deliverables.md).
- `docs/` is this tree.

## What's *not* in the repo

- The 446 comic page scans. Those live in `external/` (gitignored or LFS-managed; decision in Phase 0). Transcripts go in `lore/canon/comics/` and *are* committed.
- The Council of Castaka source. Linked via the `external/references/` directory but not vendored.
- Honcho's hosted state. Lives in Honcho's hosted backend.

## What links the Go and Python sides

- HTTP over `127.0.0.1`. The Python sidecar binds local-only by default.
- `Makefile` boots both with one command (`make dev`).
- Docker Compose for local integration testing with optional Qdrant.
- Production deploy: single Docker image containing both binaries. See [stack.md](stack.md).

## Repo bootstrapping

Phase 0 deliverable: a `scripts/init.sh` that:

1. Verifies Go 1.22+ and `uv` are installed.
2. Initializes the Go module and Python venv.
3. Symlinks `external/references/council of castaka/metabarons/` into `lore/actors/` (so the cards are the source of truth, not duplicated).
4. Runs `seed-honcho` against a fresh workspace to verify end-to-end.
