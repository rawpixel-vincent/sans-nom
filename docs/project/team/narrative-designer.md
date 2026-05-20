# Narrative Designer

## Owns

- The Narrator's [peer card](../../architecture/narrator-agent/peer-card.md). The single most load-bearing artifact in the project.
- All [actor card seeding](../../architecture/actor-agents/card-seeding.md) from accepted lorepack artifacts.
- Lorepack scenario contracts: synopsis, protagonist role, opening pressure, must-honor rules, must-not-violate rules, and choice principles.
- The [regression scene corpus](../../phases/phase-0-preproduction/deliverables.md). Hand-authored test scenes that become the eval set.
- Lore consistency QA. Reads beta playthrough samples, surfaces contract/canon violations, drafts prompt or pack revisions to address them.
- The [glossary](../../glossary.md) and [IP commitments](../../mechanics/world/ip-commitments.md). The reference set the Narrator and the team consult.

## Collaborates with

- **Lead engineer** on prompt-template changes that span code + content (e.g., the [interpretation prompt](../../mechanics/input/interpretation-prompt.md)).
- **Producer** on beta playthrough review, scene-quality grading, what to flag for revisits.
- **Frontend engineer** on prose-pane typography, italic interiority rendering, scene pacing UI.

## Phase responsibilities

- **Phase 0** — Narrator card v1, lorepack contract authoring, regression corpus authoring, actor card seeding pass, comic OCR review.
- **Phase 1** — Narrator card iteration based on beta playthrough data, regression corpus expansion as new failure modes surface.
- **Phase 2** — Bushitaka training arc scene authoring + designer review, NPC density grading, image-style reference curation.
- **Phase 3** — chest-crest scene authoring (each ancestor speaking *to* Sans-Nom), Aghora-mercy revelation arc, Steelhead-encounter variations.

## Hiring profile

- Has written narrative for games or comparable interactive media. Understands the prose-vs-game-text distinction.
- Deep familiarity with the Metabarons IP, OR willingness and aptitude to develop it. The team is small; surface-level fan engagement is not enough.
- Has worked with LLM-driven narrative tooling, OR is comfortable iterating on prompt design as authorial craft.
- Cares about voice and tone above all else. Can articulate why a scene reads wrong without falling back to vague "it doesn't feel right."

## Anti-pattern

A designer who treats the Narrator card as a rule list to populate, rather than a voice to author. The peer card is a *character* — the writer who would never let Othon beg. See [Pillar 3](../../principles/pillars/03-narrator-as-author.md).
