# Pillar 2 — Characters know what they have witnessed and nothing more

Honcho's directional representations are the engine. No character ever knows something they shouldn't. The dynasty's defining feature — that every Metabaron carries refracted understanding of every other — becomes the system, not a writer's flourish.

## What this means in practice

- Each character's view of every other character is a *separate object* in Honcho. Othon's representation of Sans-Nom is not Honorata's representation of Sans-Nom.
- A character only updates its representation of another character when it has *witnessed* that character — directly, through a recording, through testimony, or (in late-game) through the chest-crest.
- Tonto knows what Tonto saw. Aghora knows what Aghora saw. The Narrator is the only entity that sees everything, and it does not leak that omniscience into characters' voices.

## What this rules out

- Characters speaking with knowledge they could not have. If Honorata references something only Steelhead witnessed, the scene is broken.
- Shared "world state" that all characters read from. There is no global truth that characters consult; there are only their own representations.
- Convenient revelations ("Aghora suddenly remembers…"). Information moves between characters by witnessing, not by Narrator fiat.

## What depends on this

- The [Honcho integration](../../architecture/honcho-integration/representations.md) — `observe_others=true` at session level is the mechanic that produces the directional structure.
- The [cast subtree](../../mechanics/cast/) — every character file declares what that character *has witnessed* across the saga, which is the seed for their representations of others.
- The [Tonto-as-storyteller mechanic](../../mechanics/cast/tonto.md) — Tonto narrates from his own representations, not from canon. When he tells you about Edna, you are getting Tonto's Edna, not the canonical Edna.
- The [chest-crest mechanic](../../mechanics/cast/dead-and-absent.md) (Phase 3) — the only architectural exception, where ancestor representations of Sans-Nom become readable to Sans-Nom.
