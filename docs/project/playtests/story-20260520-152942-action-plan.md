# Action Plan: story-20260520-152942 Follow-Up

Date: 2026-05-21

This plan turns the run analysis into implementation units. It is intentionally scoped to preserve what is working in the current system: bounded packets, Writer/Narrator split, scene-scoped Honcho memory, agent workspaces, and lorepack scenario contracts.

## Implementation Status

- Priority 0 implemented: terminal handoff Writer refresh now falls back to the displayed branch/player decision when the submitted continuation is empty. Raw terminal Enter no longer fabricates `wait`.
- Priority 1 implemented: Writer context now includes `STORY PHASE GUIDANCE`, derived spent motifs, required next movement, and a servant-memory proof warning when witness/testimony plus servant/household motifs have been spent. Writer refresh also rejects a packet whose entry choices only preserve, copy, hide, carry, or revalidate the same spent proof instead of introducing macro pressure.
- Priority 2 implemented: lorepack gameplay modes are now explicit. Metabarons is labeled `canon_internal_original`, with `canon_adaptation` and `homage_original` reserved as distinct modes.
- Priority 3 implemented for Narrator scoping: Narrator agent workspaces receive compact staged lore files (`lorepack/contract.md`, `lorepack/relevant-excerpts.md`, `lorepack/source-anchors.md`) instead of the full lorepack tree.
- Priority 4 implemented: Scenarist `relevant_paths` must resolve to existing staged files; legacy `packets/<id>.json` references are mapped to `story/current-packet.json`.
- Priority 5 partially implemented: StoryBible now supports optional phase fields (`current_arc_phase`, `next_required_revelation`, `next_required_opposition`, `spent_motifs`, `open_threads`) and Writer context renders them.
- Priority 6 remains open.

## Priority 0: Fix The Writer Decision Signal

Problem:

Terminal handoff continuation can call Writer refresh with an empty decision, causing the Writer context and metadata to say `observe` even though the selected branch was real.

Evidence:

- `data/stories/story-20260520-152942/model-runs/writer/turn-000003/input.txt`
- `data/stories/story-20260520-152942/model-runs/writer/turn-000005/input.txt`
- Both show `LATEST ROLLOUT AND DECISION` as `observe`.

Implementation steps:

1. In `cmd/cli/main.go`, update the TUI terminal-handoff refresh path.
2. When the current rollout node has no choices and the submitted decision is empty, call Writer refresh with `decisionForWriterRefresh(c.state)` or equivalent last-real-decision fallback.
3. Preserve free-text terminal handoff behavior: if the user types a direction, pass the typed direction instead of the fallback.
4. Apply the same fallback to the raw play loop if it has the same terminal handoff path.
5. Add regression coverage:
   - terminal branch selected by choice is persisted;
   - player presses Enter on terminal handoff;
   - Writer refresh input shows the prior real choice, not `observe`.
6. Update writer metadata expectations so `latest_decision` reflects the branch choice or explicit free text.

Acceptance:

- A branch like `choice_carry_the_mask` appears in `LATEST ROLLOUT AND DECISION`.
- Writer metadata for the refresh has `latest_decision: "choice_carry_the_mask"`.
- Recent canon still includes the selected choice as a separate witnessed fact.

## Priority 1: Add A Story Phase Gate After Servant-Memory Packets

Problem:

The current run is strong but risks becoming a servant-memory loop. By turn 6, most choices still manage the same witness/memory resource rather than moving to weapon claim, rival pressure, or family authority.

Implementation steps:

1. Add a `story_phase` or `arc_position` field to story state or Writer context.
2. Derive the phase from the StoryBible `story_arc` and current packet history.
3. Render the next required phase in Writer context before lore references.
4. Add a phase gate to Writer prompt:
   - after `Servant schism`, the next non-terminal packet must introduce `Weapon claim`, `Public contest`, `Rival claimant`, or `Lineage revelation pressure`;
   - do not spend more than one packet on the same proof mechanism unless the player explicitly chooses delay or concealment.
5. Add verifier checks for repeated proof mechanisms:
   - reject a new packet if all three choices are variations of carrying, preserving, copying, or recording the same proof unless the StoryBible phase says this is the active phase.
6. Add a small test fixture using the current story shape:
   - prior packet centered on living witness and servant memory;
   - next packet that only offers memory spreading/ledger/hiding should get a warning or rejection unless it also introduces a new macro pressure.

Acceptance:

- After turn 6, Writer cannot produce another packet whose only dramatic resource is servant testimony.
- The next packet must expose a weapon schedule, rival claimant, family authority, or concrete older-order clue.

## Priority 2: Split Lorepack Modes Explicitly

Problem:

The current `faithful_canon_prototype` mode is contract-faithful but not canon-story-faithful. It produces a new Saryn/Sava scenario inside Metabarons pressure. That may be desirable, but it should not be mislabeled as direct canon adaptation.

Implementation steps:

1. Extend lorepack contract with explicit mode semantics:
   - `canon_adaptation`: play a concrete authored canon story line or episode.
   - `canon_internal_original`: new playable episode inside canon constraints.
   - `homage_original`: original setting using source influence.
2. Rename the current Metabarons mode or scenario to `canon_internal_original` unless we author a concrete canon-adaptation synopsis.
3. Add optional `canon_anchor` fields:
   - `source_volumes`
   - `source_events`
   - `source_characters`
   - `must_include_canon_facts`
   - `allowed_original_insertions`
4. For a direct Metabarons prototype, write at least one concrete scenario contract from a source arc:
   - Othon receives Castaka inheritance and the epyphite secret becomes war pressure.
   - Aghnar undergoes the crushed-feet initiation and later faces the father-kill tradition.
   - Aghora's body/lineage crisis and the birth of No-Name.
   - No-Name faces patricide and the end of the dynasty.
5. Update seed prompt to state whether the Writer is adapting a canon episode or staging an original internal episode.

Acceptance:

- The pack tells the Writer whether it can invent Saryn-like protagonists.
- If the pack says direct canon adaptation, the seed cannot invent a new central premise without rejection.

## Priority 3: Tighten Narrator Lore Tool Use

Problem:

Narrator reads broad lorepack files during rollout. This is allowed in faithful mode, but it is noisy and can pull the scene toward source-summary texture rather than immediate staged consequence.

Implementation steps:

1. Update Narrator prompt:
   - use provided lore context first;
   - read lorepack files only for a concrete missing source detail;
   - prefer specific staged paths over full `wiki/lore.md` or large manifests.
2. Add agentfs staged reference files for Narrator:
   - `lorepack/contract.md`
   - `lorepack/relevant-excerpts.md`
   - `lorepack/source-anchors.md`
3. Do not expose huge manifests by default to Narrator unless image/page lookup is explicitly needed.
4. Keep Writer and LoreExpert broader access if needed, but make broad reads visible in logs as a warning-level diagnostic when they exceed a size threshold.
5. Add a test asserting Narrator context mentions specific staged lore excerpts and does not instruct broad lore browsing.

Acceptance:

- Normal Narrator turns no longer call `cat lorepack/wiki/lore.md`.
- The Narrator still receives enough lore context to stage Metabarons pressure.

## Priority 4: Fix Scenarist Relevant Path Staging

Problem:

Scenarist attempted to read a durable packet path that was not staged in its scoped workspace.

Implementation steps:

1. In `ask_scenarist` request construction, ensure `relevant_paths` are workspace-relative staged paths only.
2. If a durable path is selected, copy or map it into the agent workspace and return the staged path.
3. Reject or rewrite paths such as `packets/<packet>.json` when only `story/current-packet.json` is staged.
4. Add a regression test:
   - Writer passes current packet to Scenarist;
   - Scenarist can read it as `story/current-packet.json`;
   - no "File does not exist" appears for packet path.

Acceptance:

- Scenarist tool calls never see non-staged durable story paths.
- Backstage artifacts still record durable links for admin inspection.

## Priority 5: Improve StoryBible Phase Specificity

Problem:

The StoryBible has a good `story_arc`, but it is still too loose about what concrete evidence, opponent, and location should appear next.

Implementation steps:

1. Add optional StoryBible fields:
   - `current_arc_phase`
   - `next_required_revelation`
   - `next_required_opposition`
   - `spent_motifs`
   - `open_threads`
2. Update Writer refresh to carry forward these phase fields or derive them into context.
3. Add a compact "spent motifs" tracker from recent turns:
   - e.g. witness masks, living testimony, service hatch, servant ledger.
4. In Writer prompt, tell the Writer to avoid making a spent motif the core of the next packet unless the player explicitly selected that repetition.

Acceptance:

- The Writer has a visible list of what it should not center again.
- The next packet in this run would be pushed toward key-blade, weapon schedule, rival claimant, or family authority.

## Priority 6: Add A Playtest Analysis Command

Problem:

This analysis required manual shell queries across state, turns, packets, logs, and prompt artifacts. It should become a repeatable dev diagnostic.

Implementation steps:

1. Add a read-only CLI command, for example:
   - `go run ./cmd/cli -story-id <id> -playtest-report`
2. Report:
   - current state
   - turn chain
   - packet chain
   - pending turn
   - Writer refresh metadata
   - tool errors
   - lore search stats
   - Honcho commit stats
   - repeated motif counts
3. Write output to `data/stories/<story_id>/reports/playtest-report.md`.
4. Keep it read-only except for the report artifact.

Acceptance:

- A future turn-5 analysis can be generated in one command and then annotated manually.

## Suggested Next Story Direction For This Save

If continuing `story-20260520-152942`, the next Writer refresh should force the story out of pure servant testimony.

Recommended branch target after turn 6:

- If `choice_spread_breath`: the memory spreads but mutates. A rival claimant uses the altered version to accuse Saryn of staging servant rebellion. The next packet should move to a weapon schedule or rival challenge.
- If `choice_mark_ash`: Saryn gains servant-level proof, but the ash mark lets household bureaucracy track them. The next packet should introduce a family or Steward-aligned body-law champion.
- If `choice_send_witness`: Saryn gets closer to the old order, but owes the servant levels. The next packet should reveal a weapon locker schedule with a missing authoring seal and push toward key-blade/house weapon claim.

Hard rule:

Do not make turn 7 followed by turn 8 another debate about whether the mask testimony is valid. The testimony has been spent. Make someone act on it.
