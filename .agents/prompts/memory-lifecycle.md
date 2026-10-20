# Memory lifecycle pass

You are any coding agent (Claude Code, Codex, Cursor, …) asked to maintain the shared agent memory store at `.agents/memory/`. Read `.agents/README.md` first for the unit format, anchor rules and admission test. Work through the phases in order. Do NOT commit; leave the diff in the working tree and report at the end.

Structural validation, including anchor resolution, always covers the whole store because it is deterministic and cheap. Semantic verification is bounded by default so this routine scales as useful memory grows. A bounded pass reviews changed, risk-sensitive and rotating entries; a full sweep stays deliberate and occasional. Neither mode adds a producer-side gate.

## Phase 0: choose the semantic review set

Read the scheduled task's durable run memory when the harness exposes it. The successful-run checkpoint has:

- `last_successful_head` and `last_successful_date`
- `last_full_sweep_date`
- `rotation_after` (the last filename the rotation queue reviewed)

Do not update the checkpoint until the final validator passes. Treat run memory as scheduler state, not shared repository knowledge. If it is unavailable, say so in the report and use Git history plus `memory:find --all --memory`, without creating a new repository file.

Use **full-sweep mode** only when the requester explicitly asks for it or no successful full sweep is recorded within the past eight weeks. A full sweep reviews every shared entry, performs a complete dedupe scan and resets the rotation cursor.

Otherwise use **bounded mode**, with at most 24 distinct shared entries selected in this order:

1. **Changed queue, up to 12.** Entries added or changed in the working tree or since `last_successful_head`, entries whose anchors the validator reports dead, and entries `bun run memory:drift --memory` lists because their anchored lines changed since their baseline. Prefer dead-anchor entries, then drifted ones, then newly added ones, then the oldest change not yet reviewed. If the checkpoint is not reachable from the current branch, use the merge base or the last successful date and report the fallback; do not invent ancestry.
2. **Risk queue, up to 6.** Project entries with explicit `OPEN`, `ACTIVE`, pending, next-step, unmerged, deferred or live-state claims, or similar time-sensitive ones. Prioritize pending or next-step claims older than about two months and the oldest explicit verification dates.
3. **Rotation queue, up to 6.** Continue alphabetically after `rotation_after`, wrapping once at the end. This keeps quiet reference entries from never being revisited.

Deduplicate the queues. Unused slots flow to the next queue; after the rotation queue, spend any remaining capacity on deferred changed entries, then risk entries. Never exceed 24. Record deferred filenames and counts so the next run can prioritize them. Running `memory:find`, `memory:drift` and targeted `rg`/Git commands to build the queues does not count as semantic review; opening and judging an entry does.

## Phase 1: validate

Run `bun run memory:check`. Fix every error:

- Frontmatter shape, name ↔ filename, unknown sections and agnostic violations: fix them in the entry.
- **Dead anchors**: run `rg` for the symbol to find where the code moved and update the row. If the code is gone, the facts anchored to it may be gone too: re-judge the entry in Phase 2 (update it, or delete it when the fact died with the code).

Resolve warnings where it is cheap. Trim an overlong description or summary. Fix keyword counts. For a dangling `[[link]]`, fix the target name, write the missing entry if the store's context holds the knowledge, or unbracket the text if it points at an agent's private note.

Validation findings are sanitation work for this routine, not reasons to block the producer that wrote a useful memory unit.

## Phase 2: staleness and correctness

In full-sweep mode, review every entry. In bounded mode, review only the Phase 0 selection plus entries this pass changed or created. Judge each selected entry from the file itself and quick repo checks (the anchors, `git log`, whether the referenced script or flag still exists):

- **Prose entries** (the validator's `legacy prose` warnings): convert them following `prompts/agentic-format-conversion.md`. The conversion counts as their review.
- **Converted entries**: the validator proves the anchors resolve. `bun run memory:read <entry>` shows the hits and what touched them since the baseline; confirm the anchors still point at what the unit claims, then judge each Not-in-code bullet against the admission test and the current code. When every claim holds, set `verified: <today> <short HEAD>` so the next drift report starts from here.
- **Shipped or complete project entries**: keep the outcome, the decisions with their reasons and every trap. Replace play-by-play narration with a short "how it ended" line. Keep anything a future agent needs in order not to repeat a mistake.
- **Entries the current code contradicts**: fix the fact or delete the entry. Verify before deleting: check the code, not just the date.
- **Pending or next-step claims older than about 2 months**: verify them against the repo, then mark them resolved or still open with today's date.
- Never delete an entry only because it is old. Age is a prompt to verify, not a verdict.

Do not open the rest of the corpus just to make the pass feel exhaustive. Deferred entries stay queued; they are not presumed correct or stale.

## Phase 3: harness memory consolidation

If the current agent harness exposes an agent-native or private memory store, inspect only entries scoped to this repository. In bounded mode, inspect at most 12 new or updated repository-scoped candidates since the previous successful checkpoint. If the harness cannot expose a delta, take a deterministic alphabetical or oldest-first slice and record its cursor and deferred count. In full-sweep mode, inspect all repository-scoped candidates.

Compare the candidates with the shared entries that `memory:find` returns for their terms:

- Import durable, non-obvious repository knowledge that is missing from the shared store. Verify it against the current repository first, and write it in the unit format: anchors for what the code shows, Not in code for what it cannot. Flag uncertain or time-sensitive claims instead of copying them as facts.
- Merge overlapping knowledge into the best existing shared entry. Create a new entry only when the knowledge is distinct and meets `.agents/README.md`.
- Make imported knowledge agent- and environment-agnostic. Do not import personal preferences, personal names, secrets, credentials, absolute user paths, machine-specific setup or unsupported claims about remote state.
- Treat the harness memory as read-only: do not edit or delete it during this pass.
- If no harness memory is available or accessible, record that in the report and continue.

New or changed shared entries produced here join the current pass's semantic review, even when that takes the reviewed set past its initial selection. Report that expansion separately; do not use it to pull unrelated deferred entries into scope.

## Phase 4: dedupe and merge

In bounded mode, compare each selected, new or changed entry with its `[[links]]`, with `memory:find` on its title and keywords, and with `memory:find --at` on its anchored files (entries bound to the same code). Merge overlaps into the most complete file, keep the better name, delete the other file and update any `[[links]]` pointing at the removed name. Do not read every entry body for a global dedupe.

In full-sweep mode, scan `memory:find --all --memory` and the corpus for overlapping entries.

## Phase 5: header hygiene

The header is what `memory:find` matches and prints. For selected, new or changed entries: the title is specific, the description (the hook) matches the content and stays within 160 chars, and the keywords add search terms that neither the title nor the anchors carry. In full-sweep mode, check every header this way.

## Phase 6: agnostic sweep

The validator scans the complete store for common macOS, Linux and Windows user-profile paths plus the personal names listed in `agnostic.names` (`.agents/shared-memory.json`). Source-backed standardized infrastructure identities and paths are allowed when their exact value is the durable contract; the homes listed in `agnostic.allowedHomes` are such contracts, not personal machine paths. Also read selected, new or changed entries for machine-specific binaries or PATH details, pronouns tied to a person, "my machine"-style phrasing and other context a regex cannot judge. In full-sweep mode, run that prose-level check across every entry.

Move genuinely machine-local pointers out and note them in the report, so the requesting user can keep them in their agent's private store.

## Phase 7: final gate, checkpoint and report

Run `bun run memory:check` again; it must finish with 0 errors. Only after it passes, update the scheduler's durable run memory: the successful HEAD and date, the full-sweep date when applicable, the shared rotation cursor and any harness-candidate cursor. Do not write scheduler checkpoints into `.agents/memory/`.

Then report:

- mode (bounded or full sweep) and the reason
- changed, risk and rotation selection counts, the reviewed filenames, the deferred counts and filenames, and the next cursor
- entries updated, converted, merged or deleted, each with a one-line reason
- dead anchors repaired, and entries re-judged because their code disappeared
- harness-memory facts imported, merged, skipped or deferred, each with a one-line reason, or a note that no harness memory was available
- facts flagged for human verification because the repo could not confirm them
- machine-local content moved out, if any
- validator status before → after, and the `memory:drift --memory` count before → after
- whether the checkpoint was written, or why durable run memory was unavailable

Leave everything uncommitted for review.
