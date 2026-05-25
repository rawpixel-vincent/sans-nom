# Lore Retrieval And Lore Authority

Lore access now serves lore-faithful play. The active lorepack is not just a
search corpus; it is an authored gameplay contract.

## Source Of Truth

`lore/packs/<pack_id>/lore-pack.json` defines:

- retrieval metadata;
- path to the required master summary, normally `LORE.md`;
- authored `game_synopsis`;
- explicit `gameplay_mode` (`canon_adaptation`, `canon_internal_original`, or
  `homage_original`);
- `protagonist_role`;
- `opening_situation`;
- `initial_pressure`;
- `story_lines`;
- `must_not_violate`;

In scenario-contract modes, seed generation and Writer refresh use that context
for source fidelity and playable pressure. The LLM may stage, compress, branch,
and narrate the material; it should not replace the premise.
`canon_internal_original` means a new playable episode inside canon constraints;
`canon_adaptation` means a concrete authored canon episode with explicit anchors.

## Model-Visible Lore Files

Writer, Narrator, and Scenarist tools do not receive raw repo-root lore paths.
The active pack is staged into the current agent workspace as:

```text
lorepack/
```

That staged folder is the model-visible way to inspect pack contract/reference
material. Raw paths such as `lore/packs/metabarons/...` remain blocked by the
agent filesystem policy.

## LoreExpert / Lore Authority

LoreExpert is the source-grounded role for focused questions:

- Does this choice violate the pack's pressure logic?
- Which source pattern matters for this scene?
- Is the Narrator drifting into generic fantasy or unsupported invention?

In older influence-mode experiments, LoreExpert was "influence-only." In
scenario-contract modes, it is better understood as lore authority and reference
support.

## Retrieval At Turn Time

Runtime retrieval should be scoped by:

- selected lorepack id;
- current scenario/arc position;
- current packet node;
- player decision or free-text;
- present actors and witnessed context.

Results feed Writer/Narrator context as reference and constraint material. They
do not replace the authored scenario contract.

## Master Lore Summary

Each active lorepack must include `LORE.md` at the pack root, or an equivalent
path declared as `artifact_roots.summary` in `lore-pack.json`. This file is the
final human-readable build artifact for the pack: a thorough, checked summary of
the source lore, character lines, pressure systems, and hard boundaries.

Runtime agents receive the content of this file in their system prompt when the
active pack is available. This is intentional: the master summary is stable
prefix context, so model backends can cache it across Writer, Narrator,
Verifier, Actor, Scenarist, and LoreExpert calls. Agents should treat it as
background truth for lore fidelity, not as text to recite.

`lore-index build` also indexes `LORE.md` as `master_lore`; missing or empty
master lore is a build failure.

## Scenarist Boundary

Scenarist can read request-specific staged paths and can use `lorepack/` when
the Writer explicitly includes it. Scenarist remains an idea/critique/staging
role, not the source of lore truth.

When source grounding is central, ask LoreExpert rather than asking Scenarist to
infer canon from broad context.

## What Lore Access Does Not Do

- It does not expose the whole repository to model tools.
- It does not make old seed runs or static fixtures model-visible.
- It does not replace Honcho representations for actors.
- It does not turn unused Writer branches into canon.

## Future Corpus Work

Comic transcript OCR, vector indexing, and per-turn retrieval still matter, but
they sit under the lorepack contract instead of acting as a loose inspiration
pool. See the lorepack master plan in
[project/lore-faithful-engine-master-plan.md](../project/lore-faithful-engine-master-plan.md).
