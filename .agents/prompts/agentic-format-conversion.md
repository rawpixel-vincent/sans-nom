# Agentic format conversion pass

You are any coding agent asked to convert legacy prose units to the agentic format: memory entries (`.agents/memory/`). Read `.agents/README.md` first: its unit format, anchor rules and admission test are the contract. Do not commit; leave the diff in the working tree and report at the end.

**Goal:** a unit a future agent can act on in one read. The anchors bind the unit to the code its facts are about, Not in code holds what the code cannot tell, and a summary orients only when the bullets need one. The test for every dropped sentence: _could an agent recover this from the anchors and the code within a couple of minutes?_ If yes, drop it. If no and it would change what the agent does, keep it as a bullet.

## Phase 0: pick the batch

List what remains from the `legacy prose` warnings of `bun run memory:check`. The requester names the system and the batch; default to at most 10 units per pass, in the order listed. Units about one subsystem convert better together, because shared anchors and merges become visible.

## Phase 1: inventory the claims

Read the whole unit. Sort each claim into exactly one bucket:

1. **Locatable**: the code states it (a signature, a default, a map, a flow inside one module, a response shape, a prop list, a file list). It becomes an anchor, or disappears when an existing anchor already covers it.
2. **Admissible**: it passes the admission test (why, trap, external, state, intent). It becomes one Not-in-code bullet.
3. **Noise**: narration, superseded attempts, restatement, counts that no decision depends on. Drop it; git keeps it.

Historical numbers (benchmarks, costs, incident timings) stay only when they justify a decision or serve as a baseline someone will compare against.

Never invent a rationale. A "why" must come from the unit's own text, a commit message or a code comment you actually read. If none of them states the reason, keep the fact without a why, or drop it. Anything a code comment already explains is locatable, not admissible.

## Phase 2: find the anchors

- For each locatable cluster, find the definition site with `rg` and write one anchor: a regex that hits the definition or a unique string, and the narrowest stable glob. One anchor per concept; join sibling symbols with alternation.
- Run `bun run anchors <file.md>` and read the hits. Every anchor must resolve, and each hit must be the thing the summary or note claims. A pattern that matches dozens of call sites is too broad.
- When a claimed symbol no longer exists, the claim is stale: find its replacement, or treat the claim as dead.

## Phase 3: verify what you keep

- Check each code-facing Not-in-code bullet (a trap, a divergence, state that names code) against the current code while you are at the anchor. Correct it or drop it. Record every corrected or dropped claim for the report.
- External facts (incidents, measurements, vendor behavior, manual deploys) cannot be verified from the repo. Keep them with their original date and evidence reference.
- Open or pending items older than about 2 months: check `git log` and the code, then mark them resolved or still open with today's date.
- Work offline. The clone may be shallow: history older than the local `git log` is unverifiable, so keep such claims with their original date and evidence. Never fetch history or content over the network (no GitHub API, no `curl`, no `git fetch`).

## Phase 4: write the unit

- Frontmatter per `.agents/README.md`. `title`: keep the unit's current title (its H1) unless it is wrong. `description` is the hook `memory:find` prints: at most 160 chars and specific. `keywords`: 2–8 search terms carried by neither the title nor the anchors (synonyms, product vocabulary, external names).
- Summary: optional, at most 3 sentences, only what a reader needs before the bullets. For dated docs (audits, plans, incidents), give the date, the commit or build it concerns, and the verdict or status (shipped, partial, parked, superseded).
- `## Anchors`, then `## Not in code`. Bullets are one fact each, in at most 2 lines. Use `###` steps inside Not in code only for operator procedures that no script encodes.
- Keep filenames stable; other files link to them. Before merging or deleting a unit, run `rg -l '<filename>'` and update the inbound links.
- Memory: merge an entry into an overlapping one when conversion shows they are one subject (update the `[[links]]`). Do not move knowledge into the repo's docs during conversion; list those candidates for `memory-docs-promotion.md` instead.

## Phase 5: gate and report

Run `bun run memory:check`. There must be 0 errors.

When several agents convert batches in parallel, each touches only its own files. Validate with `bun scripts/agents-anchors.ts <file>` and with the checkers filtered to your files.

Then report:

- the units converted, with chars before → after and the anchor and bullet counts
- claims corrected or dropped as wrong, one line each with the evidence
- claims kept but flagged (external or unverifiable, or unsure whether the code shows them)
- merges, deletions and link updates
- promotion candidates, and the remaining prose count

Leave everything uncommitted for review.
