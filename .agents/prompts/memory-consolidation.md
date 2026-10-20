# Memory consolidation and compaction pass

You are any coding agent asked to keep the shared memory store at `.agents/memory/` cheap to READ. Read `.agents/README.md` first; its admission test is the rule this pass enforces. This pass is only about read and token efficiency; the sibling `memory-lifecycle.md` owns correctness and staleness. Do NOT commit; leave the diff in the working tree and report at the end.

**Ground rules:**

- There is no size quota. Cut a sentence because a future agent can get it from the anchors and the code, or does not need it to act correctly, never because the entry is long. When you are unsure whether the code shows something, keep it and flag it in the report.
- Git history keeps every old version. The store's job is to transfer knowledge in the fewest words, with anchors that take the reader into the code.

## Phase 1: triage

List entries by size (`wc -c .agents/memory/*.md | sort -n`) and prose status (the `legacy prose` warnings of `bun run memory:check`). Work the heavy tail and the prose entries first, but judge each entry on its content, not its bytes. A long Not-in-code section full of incidents and reverted designs may be perfect; a three-line entry that restates obvious code is pure waste.

## Phase 2: apply the admission test

For every Not-in-code bullet (and all prose in a legacy entry):

- **Paraphrase of code in one readable place**: replace it with an anchor to the definition, or drop it when an existing anchor already covers it. The code is canonical; prose summaries of readable code go stale and cost tokens twice.
- **Logic scattered across files**: when the scattering itself is the trap (coupling between packages, an ordering ritual, behavior visible only across several files), keep one tight bullet and anchor each end. Otherwise anchors on each piece are enough.
- **An entry that only restates what its anchors show**: reduce it to its anchors, with a summary only if a reader needs it before the bullets, or delete it when the hook and one anchor would do.
- Convert legacy entries to the format with `prompts/agentic-format-conversion.md`.
- Before adding or changing an anchor, confirm it resolves: `bun run anchors <entry.md>`.

## Phase 3: compress finished narratives

For shipped or complete work, keep the outcome, the final location (as anchors), decisions with their reasons and every trap. Drop phase-by-phase session narration, superseded attempts and resolved dead ends. What survives is "how it ended + what still bites".

## Phase 4: merge fragments

Several partial entries about one subsystem cost more to discover and read than one complete entry. Entries sharing anchor globs or keywords are candidates. Merge them into the best file, keep the better name, delete the others and update the `[[links]]`.

## Phase 5: header efficiency

The header (title, description, keywords) is what `memory:find` matches and prints for every agent. Tighten the headers you touched: a specific title, a hook of at most 160 chars, and keywords that add search terms carried by neither the title nor the anchors. The summary is optional: one that paraphrases the anchors goes.

## Phase 6: final gate and report

Run `bun run memory:check`; it must finish with 0 errors. Then report:

- store size before → after (total chars) and per-entry deltas for changed files
- passages turned into anchors (with the anchor), and bullets deliberately kept despite their length (one-line reason each)
- entries converted, merged or deleted (one-line reason each)
- anything kept but flagged as possibly derivable from the code

Leave everything uncommitted for review.
