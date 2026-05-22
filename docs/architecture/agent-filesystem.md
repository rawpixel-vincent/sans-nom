# Agent Filesystem

Sans-Nom uses a small in-repo `internal/agentfs` package to build curated,
per-run filesystem views for model tools. This is not a security sandbox by
itself; it is the policy layer that decides which files are staged, which paths
are visible, and which repo surfaces are rejected before a model-backed tool can
read them.

## Current Model

Each role receives an agent workspace rooted under the active run:

- Seed runs: `data/seed-runs/<seed_id>/agent/`
- Story runs: `data/stories/<story_id>/agent/<role>/<run_id-or-turn>/`

The workspace is populated with curated files only:

- `story/` for current story state, packet, node, and continuity material.
- `workshop/` for seed-workshop artifacts in legacy or diagnostic paths.
- `scenarists/` for Scenarist outputs that the Writer can read.
- `lorepack/` for the active lorepack scenario contract and curated reference
  files.

The raw repository root is not exposed to Writer, Narrator, or Scenarist tools.
Blocked paths include `.git/`, `cmd/`, `internal/`, `testdata/`, raw
`lore/`, unrelated `data/stories/`, and old `data/seed-runs/`.

## Why This Exists

Early prompt/tool experiments exposed too much surface area. Creative agents
could read old static fixtures, previous seed runs, test docs, and source files,
which made recurring motifs look like model taste when some of them were simply
context leaks.

The current agent workspace layer has three jobs:

1. Keep model-visible material scoped to the active story or seed run.
2. Make returned artifact paths immediately readable by the next model call.
3. Preserve admin/debug access to durable real paths without teaching models to
   use those real paths.

## Lorepack Access

Scenario-contract modes intentionally let creative roles inspect the active
lorepack, but only through the staged `lorepack/` directory. This gives Writer,
Narrator, and Scenarist direct access to the authored scenario contract without
giving them repo-root access to every pack, source file, test, and old run.

Raw paths such as `lore/packs/metabarons/lore-pack.json` should still be
rejected by model-visible tools. The model-visible equivalent is:

```text
lorepack/lore-pack.json
lorepack/LORE.md
```

`LORE.md` is the pack's required master summary. Runtime agents receive its
content in system prompt context when available, and the staged file remains
readable for explicit checks.

## External Reference: Turso AgentFS

[Turso AgentFS](https://github.com/tursodatabase/agentfs) is an external
project worth tracking. Its repository describes AgentFS as a filesystem for AI
agents and marks the software beta. Its main pieces are:

- CLI support, including host mounts via FUSE on Linux and NFS on macOS.
- SDKs for programmatic access, including a
  [Go SDK](https://github.com/tursodatabase/agentfs/blob/main/sdk/go/README.md)
  with filesystem, key-value, and tool-call APIs.
- A SQLite-backed
  [Agent Filesystem Specification](https://github.com/tursodatabase/agentfs/blob/main/SPEC.md)
  covering virtual files, key-value state, and insert-only tool-call audit
  trails.
- A [manual](https://github.com/tursodatabase/agentfs/blob/main/MANUAL.md) for
  CLI and mount workflows.

## External Reference: Mirage

[Strukto Mirage](https://github.com/strukto-ai/mirage) is another external
project worth tracking for the agent filesystem layer. Mirage provides a
virtual filesystem and shell-like command executor for agents. Its design is
closer to Sans-Nom's model-visible read tool surface than Turso AgentFS is:
agents can use familiar commands such as `cat`, `ls`, `grep`, `find`, and
pipelines, but the executor operates against mounted virtual resources instead
of the host repository.

Potential fit:

- Replacing part of the hand-rolled read-only command validation in
  `internal/codexoauth/local_tools.go`.
- Mounting only the current scoped run material, such as `/story`,
  `/lorepack`, `/scenarists`, and `/workshop`.
- Snapshotting the exact model-visible workspace for failed seed runs,
  regression tests, and prompt-eval replay.
- Exposing lorepack references as virtual files without copying broad source
  trees into each agent workspace.

Not a replacement yet:

- Mirage does not encode Sans-Nom's staging policy. The curated file selection
  and forbidden-path rules in `internal/agentfs` still need to exist.
- The current runtime is Go, while Mirage's public SDK/runtime path is centered
  on Python and TypeScript. Live integration would likely require a sidecar or
  a dedicated dev-only bridge first.
- The project is early. Treat it as a spike candidate, not a production
  dependency in the live play loop.

Recommended spike:

1. Build a Mirage workspace from one seed run using only already-curated staged
   files.
2. Mount `/lorepack`, `/story`, `/scenarists`, and `/workshop`.
3. Run representative Writer commands:
   - `cat lorepack/navigation.md`
   - `rg -n "succession|honor|wound" lorepack/wiki/lore.md lorepack/comics/volumes`
   - `find lorepack -maxdepth 4 -type f`
4. Verify forbidden surfaces remain invisible: peer-card run logs, provenance,
   `testdata`, repo source, and unrelated story or seed runs.
5. Compare Mirage snapshots and logs against current backstage/detail logs.

## Fit Assessment

Both external filesystem projects are interesting for Sans-Nom, but neither is
a drop-in replacement for the current `internal/agentfs` layer.

Turso AgentFS looks strongest as an audit and state substrate:

- Mirroring per-run agent workspaces into one queryable `agent.db`.
- Recording tool-call audit trails next to files and run state.
- Snapshotting/forking model-visible workspaces during prompt experiments.
- Giving admin/backstage a richer historical substrate later.

Mirage looks strongest as the model-visible filesystem and shell surface:

- Running familiar read/search commands against a virtual scoped tree.
- Reducing custom shell parsing in local read tools.
- Capturing and replaying exact agent-visible filesystem snapshots.

Not a replacement yet:

- Sans-Nom still needs real OS paths because Codex read tools and Claude-style
  tool sessions operate against mounted or local files.
- Neither external project encodes Sans-Nom's policy by itself; we still need
  the allowlist/blocklist logic that decides what gets staged.
- Mounting adds platform and operational complexity, especially on macOS.
- Both projects are early enough that production use should come after a small
  spike.

## Recommended Future Spike

Keep the current scoped OS workspace as the live path. Add a spike that mirrors
one seed run into Turso AgentFS:

1. Create an `agent.db` beside the seed run.
2. Copy staged `story/`, `scenarists/`, `workshop/`, and `lorepack/` files into
   AgentFS.
3. Record read-tool and model-tool events into the AgentFS tool-call log.
4. Compare admin/backstage queries against the existing JSONL/detail-log source.
5. Decide whether AgentFS becomes an audit mirror, a snapshot layer, or remains
   a research reference.

Do not replace `internal/agentfs` until the spike proves the mount/SDK path can
serve current model tools without weakening scope policy.
