# Lore Retrieval

Per-turn vector search over the canonical corpus. Feeds the Narrator the specific lore relevant to the current input + scene.

## What gets retrieved

- Canonical character cards (the 8 from `external/references/council of castaka/metabarons/`).
- Comic page transcripts (446 pages from `external/comics/meta-barons/`, ingested per the plan below).
- Existing council prompts and invocation docs (the Council of Castaka work, useful as canonical-tone references).

## Per-turn search

```go
loreSnippets := g.lore.Search(ctx, input.Text, k: 5)
```

- `k: 5` — top 5 chunks. Tunable per Phase 1 telemetry.
- Query is the player's free-text input (or a canonical version of the chosen choice option).
- Results pass into the Narrator's context bundle. See [authoring loop](narrator-agent/authoring-loop.md) step 1.

## Comic corpus ingestion (Phase 0 deliverable)

The original draft did not specify how the 446 comic pages get extracted. Specified here:

### Step 1 — Page-batched OCR via Claude vision

- Iterate `external/comics/meta-barons/` page by page.
- For each page (or 4-page batch for cost efficiency), submit to Claude vision with a prompt like: *"Transcribe all dialogue, captions, and narrator text from these comic pages. Identify panels and indicate panel transitions. Preserve speaker attribution."*
- Output: structured markdown per page or per batch, with `## Page N` headers, panel-numbered dialogue, and any visual context worth preserving.

### Step 2 — Manual canonical-quote review

- The narrative designer reviews extracted text against the actual comic pages.
- Corrections focus on canonical lines (the famous Bushitaka invocations, Tonto's opening narrations, the dynasty-defining moments). Errors in everyday dialogue are acceptable; errors in canonical quotes are not.
- Output: reviewed transcripts in `lore/canon/comics/`.

### Step 3 — Embed into vector store

- Chunk transcripts at the page or scene boundary.
- Embed using Anthropic's recommended embedding model.
- Index in Qdrant or pgvector (decision in Phase 0; both are fine).
- Output: queryable vector index that the Go server hits via the [Python sidecar](python-sidecar.md).

### Step 4 — Re-embed on file change

- File watcher rebuilds incrementally — only touched files re-embed.
- See [dev-loop ergonomics](narrator-agent/dev-loop-ergonomics.md).

## Search at turn time

- Query: player input + current scene's location/cast as context.
- Filter: by character relevance (if a query mentions a specific character, prioritize chunks about that character).
- Score: vector similarity + simple boost for recently-relevant chunks (cached per session).

## Telemetry

- Per-query: hit/miss rate, top-K score distribution, latency.
- A cold hit rate above 30% means the corpus or the embedding strategy needs work. See [observability.md](observability.md).

## What lore retrieval does *not* do

- Provide the canonical truth for the scene. The Narrator uses retrieved snippets as *reference*; it does not paste them verbatim.
- Replace actor representations. Actors get their representations of others from Honcho, not from lore retrieval. Lore retrieval informs the *Narrator*, not the actors.
- Run during actor invocation. Only the Narrator queries the lore index.
