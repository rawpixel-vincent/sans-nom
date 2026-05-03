# Sans-Nom Card Seed

The starting peer card for the player's character. Loaded into Honcho when a new playthrough begins.

```
TRAIT: I am the child of Aghora. I do not know my father.
TRAIT: I am being raised in the tradition of the Bushitaka,
       though my parent has not yet made me train.
TRAIT: I am called Sans-Nom because no one has named me. I do
       not yet know why.
TRAIT: I have no memories of the dynasty before me. Tonto tells
       me stories.
```

## What these traits do

- They give the Narrator a starting point for Sans-Nom's voice and posture without overdetermining who he becomes.
- They make explicit the things he *does not know* — these absences are the engines of the early game's curiosity.
- They name his relationship to the two characters always present at start (Aghora, Tonto) and the two structural facts (his missing father, his missing name).

## What's deliberately not in the seed

- No trait says Sans-Nom is brave, kind, curious, or anything else dispositional. Those grow from play, via [card evolution](card-evolution.md). The seed is a clean child.
- No trait references the Bushitaka decision. That is the late-game crux; Sans-Nom does not start aware of it.
- No trait references the chest-crest, his ancestors as voices, or his future role. The seed is age-appropriate.

## Source

Seed copied from §7 of the original GDD draft. Compare to the [SansNom card](../../external/references/council of castaka/metabarons/SansNom.md) used by Council of Castaka — that card is the *adult* Sans-Nom (post-Bushitaka). The two are different characters at different times of his life. The seed here is the child; the Council card is what one possible adult Sans-Nom became in the canonical comic.

## Maintenance

The seed is **immutable per playthrough**. It loads at workspace creation and is never edited during play. Evolution happens by appending or replacing facts via [card-update paths](../memory-model/card-updates.md). The seed itself stays clean so that the *evolution* — the difference between seed and current card — is the player's authored story.
