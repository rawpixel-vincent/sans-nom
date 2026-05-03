# Verb: Recall

Always available. Surfaces a memory of a past scene — verbatim or summarized, depending on how recent.

## Trigger

A persistent affordance. Player's tool for re-reading their own story.

## Behavior

Player picks a moment (recent scenes from a list; older scenes by character or location filter). The Narrator surfaces the scene either verbatim (recent) or as a brief recollection (older).

Recall is the player's read access to the Honcho session log. Implementation-side, see [Honcho integration](../../architecture/honcho-integration/sessions.md) — recent scenes are still in working session memory; older scenes are summarized via the dreaming pipeline.

## Why this exists

Long playthroughs (4–20 hours) generate prose the player wants to revisit. Without Recall, the player is stuck with their imperfect memory of what just happened to them.

It is also the only way the player can verify continuity claims. If Aghora seems to remember something Sans-Nom said three sessions ago, the player can Recall the moment and confirm — and that's how they learn that *Aghora actually remembers*, not just the Narrator pretending.

## Notes for the Narrator

- Recall is read-only. It does not advance time. It does not commit to Honcho.
- Recall *can* be in-fictional ("Sans-Nom remembers…") for recent scenes, and gradually become out-of-fictional (a transcript view) for older scenes. The exact UX is a Phase-1 design decision.
- Recall is not search. The player picks from a curated list of memorable scenes (Narrator-flagged at commit time). A search-everything affordance breaks the texture.
