# Phase 0 Deliverables

Concrete artifacts produced in Phase 0. Each is a file or set of files that a teammate can point to and say "this is done."

## Narrator artifacts

- `lore/narrator/peer-card.md` — Narrator peer card v1, with TRAIT and INSTRUCTION entries. See [peer-card.md](../../architecture/narrator-agent/peer-card.md) for the v1 draft.
- `testdata/scenes/` — **the regression corpus**: 20 hand-authored test scenes, each with:
  - The player input (free-text or chosen option) that produced the scene.
  - The reference Narrator output (what a passing run should produce, in tone and content).
  - The expected actor invocations (which characters should be queried).
  - A pass/fail evaluation pattern (typically a tone match + a canonical-fidelity check, both human-graded; the eval-narrator script automates the diff and presents it for grading).
  - 5 of the 20 are *adversarial inputs* covering [edge case categories 1–5](../../mechanics/input/edge-cases.md): canonically impossible, child-cannot-do, out-of-character demand, fourth-wall break, prompt injection disguised as character speech.

## Actor artifacts

- `lore/actors/aghora.md`, `lore/actors/honorata.md`, ... — 8 canonical character cards adapted for game use. Some characters need a *young* version for early-phase relevance (e.g., young Steelhead for stories Tonto tells; the Council version is post-everything).
- `lore/actors/tonto.md` — written from scratch, drawing on comic narration sequences and the Council `council_summoning_prompts.md`.
- `lore/actors/lothar.md` — written from scratch.
- `lore/relations/` — directional relationship matrix, loaded from the Council of Castaka `relations/` directory verbatim (the dynasty's refraction matrix is already written).

## Engineering artifacts

- `cmd/cli/` — Bubble Tea CLI testbed. Can: load a workspace; run a manual scene; replay a regression corpus scene; compare to reference; run the full corpus with a summary.
- `internal/narrator/`, `internal/actors/`, `internal/scene/`, `internal/lore/`, `internal/bridge/`, `internal/prompt/` — the Go-side agent loop. ~400 LOC for the loop itself; more for surrounding scaffolding.
- `bridge/src/bridge/main.py` + `honcho_ops.py` — the [Python sidecar](../../architecture/python-sidecar.md) with all 6 v1 endpoints.
- `scripts/seed-honcho.go` — one-shot bootstrap of a clean Honcho workspace from `lore/`.
- `scripts/eval-narrator.go` — runs the regression corpus and presents diffs for human grading.

## Lore corpus artifacts

- `lore/canon/comics/` — extracted text from the 446 comic pages. Page-batched OCR via Claude vision; manually reviewed for canonical-quote accuracy. See [lore-retrieval.md](../../architecture/lore-retrieval.md).
- A populated vector store (Qdrant or pgvector — Phase 0 picks). The `seed-honcho` script's setup phase loads the corpus once.
- **Edna lore research** — `lore/actors/edna-research.md`. The narrative designer searches the 446 comic transcripts for Edna appearances and assembles a minimum of 10 canonical observations: her voice, her relationship to Tonto, her era in the dynasty's timeline, any specific scenes or dialogue attributed to her. This file is the seed material for the Tonto-of-Edna verification below. **This deliverable is a prerequisite for the Tonto-of-Edna verification** — it must be complete before the verification script can be seeded and run.

## Verification artifacts

- A passing run of [Tonto-of-Edna generative recall verification](../../architecture/honcho-integration/representations.md). 50-line Python script with documented output. *Depends on the Edna lore research deliverable above.*
- A passing run of the regression corpus (graded "in tone" by the narrative designer for all 20 scenes).
- A demonstrated end-to-end Honcho integration via the `seed-honcho` script + a 10-scene CLI play through the toy scenario.
- A populated Honcho dashboard showing observability data from CLI playtests.

## Documentation artifacts

- This `docs/` tree, fully populated. Each Phase-0 work-stream updates the docs as it learns.
- ADRs 0001–0004 closed (status: accepted). Any new ADRs from Phase 0 work added to [`architecture/decisions/`](../../architecture/decisions/).

## Non-deliverables

- No web frontend.
- No deploy artifacts (Docker images can be built but production deploy is Phase 1).
- No image gen, TTS, monetization (later phases).
