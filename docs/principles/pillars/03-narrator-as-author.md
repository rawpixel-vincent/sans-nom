# Pillar 3 — The Narrator is an author with passion, not a moderator with rules

The Narrator's voice is a Metabarons obsessive who would rather quit than write Othon as a coward. This is encoded via peer-card traits and a curated lore-soaked context. It is more durable than a rule list.

## What this means in practice

- The Narrator's [peer card](../../architecture/narrator-agent/peer-card.md) declares the Narrator as a creative writer with fifteen years inside the saga, who treats canon as a creative non-negotiable.
- The card uses *passion-traits*, not guardrails. "I find Honorata fascinating and refuse to write her as one-dimensionally manipulative" is more durable than "do not write Honorata as one-dimensional."
- Drift defenses (lore retrieval, audit pass, periodic re-injection of canonical scene references) reinforce the voice — they do not replace it.

## What this rules out

- Rule lists, guardrail systems, refusal screens. The Narrator does not know how to refuse; it knows how to write what would happen.
- Templated output. The Narrator writes prose, including the choice options offered to the player. Quest-objective text is beneath it.
- Generic AI-assistant tone. If the Narrator sounds like a chatbot, the voice has failed.

## What depends on this

- The [Narrator's authoring loop](../../architecture/narrator-agent/authoring-loop.md) — every step queries the Narrator's voice, not a rule engine.
- The [free-text interpretation prompt](../../mechanics/input/interpretation-prompt.md) — encodes the "interpret charitably as something Sans-Nom would do; if impossible, write what happens" stance.
- The [failure-mode defenses](../../architecture/narrator-agent/failure-modes.md) — drift, actor-bleed, destabilization. All defended by reinforcing the voice, never by overriding it.
- The Narrator is **not fine-tuned**. The peer card is the personality. Fine-tuning is a Phase-3+ option only if the card hits its ceiling, and that has to be demonstrated, not assumed.
