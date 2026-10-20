# Memory → docs promotion pass

You are any coding agent asked to move stabilized knowledge OUT of the shared memory store (`.agents/memory/`) and INTO the repo's own documentation. Read `.agents/README.md` (unit format and admission test) first. This repo has no feature-spec layer: the promotion targets are its ordinary docs (`README.md` files, `AGENTS.md` for rules every agent must follow) and any destinations the repo rules in `.agents/README.md` list. This is a weekly pass: small, fully verified moves beat bulk migration. Do NOT commit; leave the diff in the working tree and report at the end.

**Ground rules:**

- Docs are source-verified. Nothing enters a doc on memory's word alone: re-verify every claim against the current repo before writing it. The memory entry is the lead; the code, config and scripts are the evidence. Claims about deployed or remote state that the repo cannot back stay out of the docs.
- Division of labor: **docs** hold stable, settled "how this works" knowledge: workflows, conventions, layouts, runbooks. **Memory** keeps everything else: in-flight project state, measurements and their dates, dev recipes, environment and workflow traps, and incident history. One entry can split across both.
- Promote at most 2 candidates per run (a new doc section or a substantial append each count as one). Prefer depth over breadth, and leave the rest as next week's queue in the report.
- When you are unsure whether knowledge is stable or doc-worthy, leave it in memory and flag it in the report. "No promotable entries this week" is a valid outcome.

## Phase 1: scan and shortlist

Run `bun run memory:find --all --memory` and skim the entries. An entry, or part of one, is a candidate when it describes how something in this repo works and that behavior has settled: no "ACTIVE", "unmerged", "deferred" or "next step" language, and `git log` on its anchored paths shows no ongoing rework. These are not candidates: outage postmortems, migration state, measurements that will be re-fitted, and harness or workflow traps. For each candidate, pick the document that already covers that area (the README closest to the code); that decides between append and a new section.

## Phase 2: verify against source

For each selected candidate (respect the cap), open the code the claims are about and confirm each one, file by file and behavior by behavior. Correct claims that do not hold or drop them. If the memory turns out to be wrong, fix the memory entry too and note it in the report. Keep the list of files you verified for the report.

## Phase 3: write the doc change

- Read the whole target document first; add to the right section, matching its structure and tone. Only your verified claims go in. Prefer commands and paths over prose, and keep the link style the document already uses.
- If existing doc text contradicts what you saw in the code, do NOT rewrite it here; flag it in the report, because a full doc review is out of scope for this pass.
- Create a new document only when nothing covers the area, and link it from the nearest existing README so it is discoverable.

## Phase 4: trim the memory side

Reduce the promoted entry to what still belongs in memory: in-flight state, traps and dev recipes, plus a one-line pointer to the document section. Delete the entry if the document absorbed everything, and fix any `[[links]]` pointing at a deleted or renamed entry. Update the entry's description and keywords.

## Phase 5: gate and report

Run `bun run memory:check`; it must report 0 errors. Then report:

- promotions made: memory entry → document and section (new or appended), with the files you verified against
- memory trimmed or deleted, chars before → after for each touched entry
- claims corrected or dropped during verification (one line each)
- doc contradictions flagged for human review
- the deferred candidate queue for next week, or "no promotable entries this week"

Leave everything uncommitted for review.
