# Lore-Faithful Engine Pivot Master Plan

## Summary

Sans-Nom is pivoting from "LLM invents an original story from lore influence" to
"the selected lorepack defines the playable story synopsis and lore contract;
LLMs stage, branch, narrate, and verify lore-faithful play."

For v1, use scenario-contract modes. Fidelity beats originality. The game must
honor the selected lorepack's story, constraints, choice logic, and dramatic
pressures. The current Metabarons scenario is `canon_internal_original`: a new
playable episode inside canon constraints, not a direct retelling of one volume.

This document was the task source for the pivot implementation units. Keep it
as the high-level operating model and use the status section below to separate
completed pivot work from future cleanup/spikes.

## Implementation Status - May 2026

Implemented:

- Lorepacks now carry authored gameplay contract fields.
- The Metabarons pack is the first `canon_internal_original` scenario pack.
- Seed prompts use the lorepack scenario contract instead of broad premise
  invention.
- Writer refresh and Narrator context include lore contract material.
- Active lorepack files are staged into scoped agent workspaces as `lorepack/`.
- Scenarist calls are sequential advisory sessions, not parallel creative
  batches.
- Admin/backstage surfaces model/tool events, local read tools, provider/model
  metadata, and artifacts.

Still active:

- Keep pruning old docs/tests/prompts that describe the pre-pivot influence
  engine as current behavior.
- Keep hardening lore-fidelity verification and choice-principle checks.
- Evaluate Turso AgentFS as a possible audit/snapshot mirror, not as an
  immediate replacement for `internal/agentfs`; see
  [agent-filesystem](../architecture/agent-filesystem.md).

## Current State

- Lorepacks are selectable and persisted through `ground_lore_pack_id`.
- `lore/packs/<pack_id>/lore-pack.json` now describes retrieval metadata and the
  authored gameplay contract.
- Scenario-contract modes treat LoreExpert as lore authority/reference support.
- Scenarist is an optional creative staging/criticism helper, not a premise
  generator.
- The Writer/Narrator split, bounded `StoryPacket`, agent-scoped filesystem,
  backstage view, and scene-scoped Honcho memory are valuable and should stay.

The main historical failure mode was clear: abstract influence prompts produced
generic motif completion. The pivot removes broad premise invention from the LLM
and makes the lorepack the story source of truth.

## Target Engine Model

The selected lorepack becomes both retrieval corpus and authored gameplay
contract.

Seed generation flow:

1. Load the active lorepack.
2. Read its authored scenario contract.
3. Build the initial `StoryBible` and three-turn seed runway from that contract.
4. Verify lore fidelity before accepting the seed.
5. Let the Narrator roll out player-facing prose from accepted packet nodes.

Runtime flow:

1. Writer refresh receives current canon, active scenario position, lore
   contract, and relevant lore references.
2. Writer branches inside the lorepack's choice logic instead of inventing a new
   premise.
3. Writer, Narrator, and Scenarist file tools read curated workspace files only.
   The active pack is staged inside that workspace as `lorepack/`; raw repo-root
   paths such as `lore/packs/...`, `cmd/`, `internal/`, old stories, and old
   seed runs remain blocked.
4. Narrator receives scene-local context plus lore-fidelity constraints.
5. Only rolled-out Narrator text and witnessed scene facts become canon.

## Lorepack Scenario Contract

Extend `lore/packs/<pack_id>/lore-pack.json` with authored gameplay fields.
Exact JSON shape can be refined during implementation, but the first pass must
support these concepts:

- `gameplay_mode`: one of `canon_adaptation`, `canon_internal_original`, or
  `homage_original`.
- `game_synopsis`: short authored description of the playable story.
- `protagonist_role`: who the player is in this lorepack story.
- `opening_situation`: the starting playable situation.
- `initial_pressure`: the immediate pressure that starts play.
- `story_lines`: a few rough story lines that define the playable arc.
- `must_honor`: lore truths, pressures, relationships, and consequences that
  every seed and continuation must preserve.
- `must_not_violate`: contradictions or drift that should fail verification.
- `choice_principles`: what choices should mean inside this lorepack.
- optional `canon_anchor`: source volumes, source events, source characters,
  must-include canon facts, and allowed original insertions.

Validation rule: an active lorepack in scenario-contract mode cannot seed a
story unless these authored fields are present and non-empty. `canon_adaptation`
also requires `canon_anchor`; `canon_internal_original` may use `canon_anchor`
to define what canon pressures it preserves while allowing new internal staging.

Metabarons is the first pack to receive this contract.

## Phase Plan

### Phase 1: Documentation And Contract Shape

- Status: implemented.
- Added this master plan and recorded the decision in the project decisions log.
- Extended the lorepack Go type with optional scenario contract fields.
- Added validation for active scenario-contract packs.
- Updated `lore/packs/metabarons/lore-pack.json` with the first authored
  gameplay contract, now labeled `canon_internal_original` because it stages a
  new internal episode rather than a direct canon adaptation.
- Added tests for loading and validating the new contract fields.

Exit criteria:

- `go test ./internal/lorepack` passes.
- An active pack missing contract fields fails validation.
- The Metabarons pack exposes the contract through `lorepack.GetPack`.

### Phase 2: Seed Engine Pivot

- Status: implemented; continue cleanup of old prompt-eval probes and docs.
- Replaced the current premise-invention seed workshop path with:
  `lorepack scenario contract -> seed prompt -> StoryBible + StoryPacket`.
- The seed Writer packages and stages the authored synopsis. It must not invent
  the central premise.
- The final seed prompt includes the scenario contract directly.
- The verifier rejects:
  - contradiction of `must_honor`;
  - violation of `must_not_violate`;
  - generic choices that do not express `choice_principles`;
  - drift away from the authored synopsis.
- Keep the existing five-node, three-turn seed runway unless a later decision
  replaces it.

Exit criteria:

- Static seed remains deterministic.
- Live seed prompt logs show scenario contract material instead of broad DNA
  premise generation.
- Seed tests prove the Writer is not asked to invent a new premise.

### Phase 3: Runtime Lore Fidelity

- Status: implemented at prompt/context level; verification can keep improving.
- Writer refresh receives the active scenario contract and current arc position.
- Narrator receives lore-fidelity scene constraints alongside current packet
  and scene context.
- Agent workspaces stage the active lorepack under `lorepack/` so Writer,
  Narrator, and Scenarist can inspect the actual pack material without seeing
  the repository root or unrelated stories.
- Choice generation and verification use `choice_principles`.
- LoreExpert is reframed as lore authority/reference support for fidelity
  checks. It can still retrieve source references, but its output is no longer
  just "influence-only" for faithful prototype mode.
- Scenarist remains optional for staging, scene criticism, and readability, not
  premise invention.

Exit criteria:

- Writer refresh tests show contract context is present.
- Narrator prompt tests show scene constraints are present.
- Tool tests show `lorepack/...` is readable from agent workspaces while raw
  `lore/packs/...` remains rejected.
- Backstage shows lore-fidelity checks as model/tool events.

### Phase 4: Cleanup And Simplification

- Status: in progress.
- Retire or remove free-form seed workshop turns whose purpose is broad premise
  generation.
- Remove "DNA summary" language where it substitutes for explicit scenario
  contract.
- Remove anti-copy/originality boundaries from scenario-contract paths where
  they conflict with fidelity. Keep them only for future `homage_original` or
  similar modes.
- Rewrite tests that assert "no raw lore injection" when the new path
  intentionally injects curated pack synopsis and contract.
- Keep `cmd/prompt-eval` as a diagnostic tool. Mark old germ/checklist probes as
  legacy attractor tests.

Exit criteria:

- No seed prompt asks for broad original premise invention in faithful mode.
- Prompt tests describe lore-fidelity behavior instead of influence-only
  behavior.
- Legacy story workspaces still load and resume.

## Next Task Units

1. **Cleanup pass**
   - Remove retired workshop prompts, old DNA summary behavior, and obsolete
     tests.
   - Preserve `cmd/prompt-eval` for regression research.

2. **Lore-fidelity verifier hardening**
   - Expand checks for `must_honor`, `must_not_violate`, and
     `choice_principles`.
   - Keep qualitative rejection for generic choices and unsupported branch
     logic.

3. **Runtime telemetry follow-up**
   - Keep surfacing lore-fidelity checks in backstage/log mapping.
   - Add dashboards or admin filters once the event vocabulary stabilizes.

4. **AgentFS spike**
   - Mirror one seed/story agent workspace into Turso AgentFS.
   - Record model read/tool events into AgentFS tool-call audit tables.
   - Compare against current JSONL/detail logs before deciding adoption.

## Compatibility And Migration

- Existing story workspaces remain readable. No migration is required.
- Old stories may lack scenario contract metadata; treat them as legacy
  influence-mode stories when resuming.
- The selected `ground_lore_pack_id` remains the stable story-state link to the
  pack.
- If multiple scenarios per pack are added later, story state should persist the
  selected scenario id. Do not add this until the first single-scenario contract
  is working.

## Testing Strategy

- `internal/lorepack`: contract load/validation tests.
- `internal/seed`: seed prompt and verifier tests for contract-driven seeds.
- `internal/prompt`: Writer and Narrator context tests for lore-fidelity
  context.
- `cmd/cli`: static seed regression and legacy resume regression.
- Full guardrail: `go test ./...`.

## Assumptions

- Scenario-contract modes are prototype design modes, not licensing decisions
  for a public product.
- The current Writer/Narrator split remains the architecture.
- Honcho receives only rolled-out canon, not unused Writer branches or planning
  material.
- Active lorepack files may be visible to model file tools only through the
  scoped agent workspace path `lorepack/`.
- The first implementation should support one scenario per lorepack. Multiple
  scenarios can be added later.
