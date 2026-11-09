# .agents — shared agent knowledge

<!-- BEGIN shared-agent-memory: managed block, change it upstream; a local edit blocks the next upgrade -->

`memory/` is the shared memory store for every coding agent in this repo (Claude Code, Codex, Cursor, …): durable, non-obvious knowledge such as recipes, traps, decisions and project state. Git is the sync layer, the durability layer and the archive.

## Principle

The code is the source of truth, and an agent reads code faster than prose about it. A unit keeps only what the code cannot say and binds it to the code it is about: an anchor is not a navigation hint but the attachment of a fact to a line, which is what lets tooling hand the fact to an agent working on that line and detect when the line changed. Paraphrasing readable code is waste: it costs tokens twice and it silently goes stale. The validators check every anchor; nothing checks prose.

## Unit format

```markdown
---
name: <kebab-of-filename>          # memory only: filename minus .md, `_` → `-`
title: <display title>
description: <hook: one line, ≤160 chars; what memory:find prints>
keywords: [<2–8 search terms>]
verified: <YYYY-MM-DD> <commit>    # optional
metadata:                          # memory only
  type: project | reference
---

<Optional summary, ≤3 sentences: only what a reader needs before the bullets.>

## Anchors

- `<grep regex>` in `<path glob>` — <optional note, ≤12 words>

## Not in code

- <one non-derivable fact per bullet>
```

- Any unit may carry `verified: <YYYY-MM-DD> <commit>`: the date and commit at which its Not-in-code claims were last checked against the code. Without it, the unit's last commit is the baseline. `bun run memory:read <unit>` prints what touched the anchored code since.
- Memory units may omit the summary and either section: an incident note may have no anchors, and a pure pointer has nothing in Not in code. No other `##` sections are allowed. When a runbook needs steps, put them under `###` headings inside Not in code.

### Keywords

`memory:find` matches a term against the title, keywords, anchor patterns and globs, hook, body and path, so keywords only need to add what a searcher would type that is in neither the title nor the anchors: synonyms, product vocabulary, external names (vendors, incidents) and package names. Use lowercase except for identifiers, and never put a comma inside a term.

### Anchors

- Each row is one ripgrep or Grep-tool query, `rg -n '<regex>' -g '<glob>'`, run from the repo root. The glob is relative to the repo root. Its hits are the lines the unit's facts are bound to: `memory:find --at <path>` lists the units whose anchors hit a file, and `memory:read <unit>` prints the hits as `path:line` (`bun run anchors <file.md>` prints them alone).
- Point at definitions or unique strings (a function name, a table name, an error message), not at call sites. Never anchor into another unit (a memory entry): its text is rewritten freely, so link it instead (`[[entry-name]]` or a relative link). Join siblings with alternation (`reserveSlot|releaseSlot`).
- Use the narrowest glob that still survives small moves: a file path, or a package subtree such as `packages/<name>/src/**/*.ts`. Brace sets are allowed.
- The regex must work in both ripgrep and JavaScript: no lookaround, no backreferences, no backticks. Matching is case-sensitive.
- Add a note only when the pattern does not show why to look. More than about 10 anchors usually means the unit should be split.
- The validators fail on a malformed row, on a glob that matches no file, and on a pattern with no hit. A dead anchor means the code moved or the fact is no longer true: find the new location, or re-judge the unit. `bun run memory:drift` lists the units whose anchored lines changed since their `verified` commit or last commit; drift is a prompt to re-judge, not a verdict.

### Not in code: admission test

Keep a bullet only if it is at least one of:

- **Why**: the reason behind a decision, or a rejected or reverted alternative and why it failed. This stops agents from reopening settled questions.
- **Trap**: behavior that a careful reader of the code would still get wrong. Examples: intentional oddities, silent failures, ordering across files or deploys, coupling between packages that no single file states.
- **External**: facts from outside the repo, such as production measurements, incidents, vendor or platform behavior, config that exists only in a console or was deployed by hand, and operator procedures.
- **State**: work that is in flight, parked or open, and known divergence between the code and its intent. Always dated.
- **Intent**: a product or business rule that the code implements but cannot justify.

Drop paraphrased code, prop, parameter and field lists, copies of constants, maps, enums and tables, file lists, styling, default values, test counts, session narration and superseded attempts. Each bullet holds one fact in at most 2 lines, uses absolute dates, and cites its evidence (commit, PR, build tag or doc path) when the claim can go stale.

Git is the archive: the prose of any earlier version of a unit stays available with `git show <commit>:<path>`, so do not keep content just in case.

## Memory store rules

- One unit per file with a kebab-case filename. `metadata.type` is `project` for ongoing work and state, and `reference` for recipes, traps and pointers. Link related entries with `[[entry-name]]`.
- There is no index. `memory:find` reads every unit's frontmatter and anchors on each call, so a new or edited entry is searchable as soon as it is saved.
- **Keep entries user- and environment-agnostic.** No personal names: write "the maintainer" when provenance matters, and use they/them. No `/Users/<name>` paths: use repo-relative paths or `~`. No machine-specific binaries or PATH details. Machine-local pointers belong in each agent's private store. Source-backed infrastructure identities and paths are allowed when their exact value is the durable contract; the homes listed in `agnostic.allowedHomes` (`.agents/shared-memory.json`) pass the guard.
- **No secrets or credentials.**
- **Update or delete stale entries instead of duplicating them.** Run `memory:find` first. Personal preferences and notes on how a user works stay in each agent's own store.
- `HANDOFF-*.md` and `PLAN-*.md` files are transient task briefs. They are exempt from the format and are deleted on completion. The name decides whether git keeps one: `HANDOFF-local-*` and `PLAN-local-*` are ignored (the kit's block in `.gitignore`); without `-local-` the brief is versioned.

## Tooling

Validation is not part of the write flow; the periodic routines fix drift.

- `bun run memory:find <term>… [--at <path>] [--memory] [--type project|reference] [--limit N] [--all]` ranks units (title, keywords and anchor patterns weigh 3, hook and anchor globs 2, body and path 1; `a|b` matches either) and prints one row per unit: path, matched fields, title and hook. `--at` lists the units bound to a file or directory through an anchor hit, alone or combined with terms: it takes a repo-relative or absolute path, keeps every anchor whose glob matches a file there and whose regex hits it, and ranks by how tightly the glob targets the path (fewest repository files behind the glob, then most hits), so an exact-path anchor outranks a tree-wide one. A file with no binding gets the count for its directory instead of a bare no-match. `--all` lists everything. `--at` is the part a grep cannot replace: a grep of the store for a file's path misses the bindings made through globs and patterns. For words, `rg` over the store works as well.
- `bun run memory:read <unit>…` prints a unit (by path, name) with each anchor followed by its hits as `path:line: text`, then the commits that touched the anchored files since the unit's `verified` commit or last commit, and which diff hunks fall within 20 lines of a hit.
- `bun run memory:drift [--memory]` lists the units whose anchored lines changed since their baseline, for the lifecycle routine.
- `bun run memory:check` validates the memory store: frontmatter, sections and anchor resolution; the memory check also runs the agnostic guard. `--no-anchors` skips anchor resolution. A unit still in prose is a warning, not an error.
- `bun run anchors <file.md>…` resolves a unit's anchors to hits.
- Before an agent edits a file, the bound-units hook (`.agents/hooks/memory-bound-units.sh`, wired for Claude Code) hands it the rows of `memory:find --at` for that file, once per file per session.
- `.agents/shared-memory.json` holds this repo's settings: the layers, frozen and generated docs, and the agnostic guard. The scripts, the hook, the routines and this block are managed by shared-agent-memory and recorded in `.agents/shared-memory.lock.json`: change them upstream and rerun its setup, because a local edit blocks the next upgrade of that file.

## Routines (`prompts/`)

Hand any of these to an agent or schedule it. Each one reports and does not commit.

- `memory-lifecycle.md`: correctness and staleness. Runs the validator, repairs dead anchors, re-checks Not-in-code claims and imports knowledge from an agent's own harness memory.
- `memory-consolidation.md`: read efficiency. Applies the admission test, turns paraphrase into anchors and merges fragments.
- `memory-docs-promotion.md`: weekly. Promotes stabilized knowledge from memory into the repo's own documentation.
- `agentic-format-conversion.md`: converts a prose unit to this format. The checkers warn about any unit that arrives in prose.

<!-- END shared-agent-memory -->

## Repo rules

<!-- Rules specific to this repo; shared-agent-memory never changes this section. -->
