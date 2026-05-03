# Free Text vs. Choices — Why Both

## The problem each solves

- **Pure free-text destabilizes the world.** The dragon problem: a player types "I summon a dragon and become emperor," and either the world bends (canonical violation) or the Narrator refuses (frame break). Neither is acceptable.
- **Pure choice-only loses the agency that justified this format.** A choice-list-only game is a gamebook, and gamebooks already exist on paper. The reason to put this in an LLM-driven engine is to give the player creative space the gamebook doesn't have.

## Why both works

The choice list is the *structural rail*. The free-text input is the *creative escape hatch*. Together:

- 2–4 written choices per scene give players a confident path through. Most players, most turns, will pick a choice.
- The free-text input gives players who want to push beyond what the Narrator anticipated a way to do that. The Narrator interprets [charitably](interpretation-prompt.md), with [edge cases](edge-cases.md) handled by writing what would actually happen rather than refusing.

## Design implications

- Choices are *not* templated. The Narrator writes them per-scene in the voice of the current narrative beat. See [verb: choose](../verbs/choose.md).
- The free-text input is *always present*, even when the Narrator has offered a strong choice list. The player should never feel railroaded.
- The contextual prompt above the input ("What does Sans-Nom say?", "What does he do?") is a small but important authoring surface. It signals what mode the moment is in.

## What this rules out

- Choice-only "rails mode" or free-text-only "sandbox mode." Both are first-class, always.
- A morality meter that tracks free-text vs. choice ratio. The Narrator does not care which one you used; only what you did.
