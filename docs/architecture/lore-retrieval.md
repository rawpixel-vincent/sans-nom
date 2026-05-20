# Lore Retrieval And Lore Authority

Lore access now serves lore-faithful play. The active lorepack is not just a
search corpus; it is an authored gameplay contract.

## Source Of Truth

`lore/packs/<pack_id>/lore-pack.json` defines:

- retrieval metadata;
- authored `game_synopsis`;
- `protagonist_role`;
- `opening_situation`;
- `initial_pressure`;
- `story_lines`;
- `must_honor`;
- `must_not_violate`;
- `choice_principles`.

In Faithful Canon Prototype mode, seed generation and Writer refresh must honor
that contract. The LLM may stage, compress, branch, and narrate the material; it
should not replace the premise.

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

- Does this branch contradict `must_honor`?
- Does this choice violate the pack's pressure logic?
- Which source pattern matters for this scene?
- Is the Narrator drifting into generic fantasy or unsupported invention?

In older influence-mode experiments, LoreExpert was "influence-only." In
Faithful Canon Prototype mode, it is better understood as lore authority and
reference support.

## Retrieval At Turn Time

Runtime retrieval should be scoped by:

- selected lorepack id;
- current scenario/arc position;
- current packet node;
- player decision or free-text;
- present actors and witnessed context.

Results feed Writer/Narrator context as reference and constraint material. They
do not replace the authored scenario contract.

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
