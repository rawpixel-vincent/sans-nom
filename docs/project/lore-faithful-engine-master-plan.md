# Lore-Faithful Engine Pivot Master Plan

## Summary

Sans-Nom is pivoting from "LLM invents an original story from lore influence" to
"the selected lorepack defines the playable story synopsis and lore contract;
LLMs stage, branch, narrate, and verify lore-faithful play."

For v1, use **Faithful Canon Prototype** mode. Fidelity beats originality. The
game must honor the selected lorepack's story, constraints, choice logic, and
dramatic pressures.

This document is the task source for the next implementation units. Keep each
unit small enough to implement, test, and review independently.

## Current State

- Lorepacks are selectable and persisted through `ground_lore_pack_id`.
- `lore/packs/<pack_id>/lore-pack.json` currently describes retrieval metadata,
  artifact roots, smoke queries, forbidden literal terms, and status.
- Seed generation still treats lorepacks as "ground-lore DNA" and asks the
  Writer to invent an original story through free-form workshop turns.
- LoreExpert is influence-only. Scenarist is a creative workshop helper.
- The Writer/Narrator split, bounded `StoryPacket`, agent-scoped filesystem,
  backstage view, and scene-scoped Honcho memory are valuable and should stay.

The main failure mode is now clear: abstract influence prompts produce generic
motif completion. The pivot removes broad premise invention from the LLM and
makes the lorepack the story source of truth.

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
3. Narrator receives scene-local context plus lore-fidelity constraints.
4. Only rolled-out Narrator text and witnessed scene facts become canon.

## Lorepack Scenario Contract

Extend `lore/packs/<pack_id>/lore-pack.json` with authored gameplay fields.
Exact JSON shape can be refined during implementation, but the first pass must
support these concepts:

- `gameplay_mode`: `faithful_canon_prototype` for v1.
- `game_synopsis`: short authored description of the playable story.
- `protagonist_role`: who the player is in this lorepack story.
- `opening_situation`: the starting playable situation.
- `initial_pressure`: the immediate pressure that starts play.
- `story_lines`: a few rough story lines that define the playable arc.
- `must_honor`: lore truths, pressures, relationships, and consequences that
  every seed and continuation must preserve.
- `must_not_violate`: contradictions or drift that should fail verification.
- `choice_principles`: what choices should mean inside this lorepack.

Validation rule: an active lorepack in Faithful Canon Prototype mode cannot seed
a story unless these authored fields are present and non-empty.

Metabarons is the first pack to receive this contract.

## Phase Plan

### Phase 1: Documentation And Contract Shape

- Add this master plan and record the decision in the project decisions log.
- Extend the lorepack Go type with optional scenario contract fields.
- Add validation for active Faithful Canon Prototype packs.
- Update `lore/packs/metabarons/lore-pack.json` with the first authored
  gameplay contract.
- Add tests for loading and validating the new contract fields.

Exit criteria:

- `go test ./internal/lorepack` passes.
- An active pack missing contract fields fails validation.
- The Metabarons pack exposes the contract through `lorepack.GetPack`.

### Phase 2: Seed Engine Pivot

- Replace the current premise-invention seed workshop path with:
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

- Writer refresh receives the active scenario contract and current arc position.
- Narrator receives lore-fidelity scene constraints alongside current packet
  and scene context.
- Choice generation and verification use `choice_principles`.
- LoreExpert is reframed as lore authority/reference support for fidelity
  checks. It can still retrieve source references, but its output is no longer
  just "influence-only" for faithful prototype mode.
- Scenarist remains optional for staging, scene criticism, and readability, not
  premise invention.

Exit criteria:

- Writer refresh tests show contract context is present.
- Narrator prompt tests show scene constraints are present.
- Backstage shows lore-fidelity checks as model/tool events.

### Phase 4: Cleanup And Simplification

- Retire or remove free-form seed workshop turns whose purpose is broad premise
  generation.
- Remove "DNA summary" language where it substitutes for explicit scenario
  contract.
- Remove anti-copy/originality boundaries from Faithful Canon Prototype paths.
  Keep them only for future `original_homage` or similar modes.
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

1. **Lorepack contract schema**
   - Extend the Go `lorepack.Pack` type.
   - Add validation for Faithful Canon Prototype packs.
   - Add focused tests.

2. **Metabarons scenario contract**
   - Author the first `game_synopsis`, `opening_situation`, `story_lines`,
     `must_honor`, `must_not_violate`, and `choice_principles`.
   - Keep it short enough to be injected into prompts directly.

3. **Seed prompt pivot**
   - Replace workshop premise invention with contract-driven seed packaging.
   - Keep the bounded three-turn runway.
   - Update seed verifier language and tests.

4. **Runtime context pivot**
   - Include scenario contract in Writer refresh and Narrator context.
   - Add lore-fidelity checks to backstage/log mapping.

5. **Cleanup pass**
   - Remove retired workshop prompts, old DNA summary behavior, and obsolete
     tests.
   - Preserve `cmd/prompt-eval` for regression research.

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

- Faithful Canon Prototype is a prototype mode, not a licensing decision for a
  public product.
- The current Writer/Narrator split remains the architecture.
- Honcho receives only rolled-out canon, not unused Writer branches or planning
  material.
- The first implementation should support one scenario per lorepack. Multiple
  scenarios can be added later.
