# Directional Representations

The architectural feature that operationalizes [Pillar 2](../../principles/pillars/02-characters-witness-bound.md). Every character's view of every other character is a separate Honcho object, updated only when that character witnesses the other.

## Honcho API surface

- `peer.set_card([...], target=other_peer)` — sets the directional card that observer `peer` holds *about* `target`.
- `peer.get_card(target=other_peer)` — reads the directional card.
- Session-level `observe_others=true` — when a session is created with this flag, every message in the session triggers updates to the present peers' representations of every other peer present. This is the engine that produces the directional structure automatically without manual `set_card` calls per scene.

## Per-scene flow

When a scene commits to Honcho:

1. The scene's message is attributed to the speaking peer (or to the Narrator if it's establishing prose).
2. `observe_others=true` propagates the message to every other present peer's representation pipeline.
3. Honcho's reasoning pipeline updates each present peer's directional representation of every other present peer they witnessed.
4. The Narrator may *additionally* trigger explicit `set_card` updates if a scene introduced stable new facts the Narrator wants to lock in (rather than waiting for dreaming to surface them).

See [authoring loop](../narrator-agent/authoring-loop.md) step 7 for the full commit sequence.

## Phase-0 verification: Tonto-of-Edna generative recall

A subtlety: Honcho's directional representations primarily surface *facts*. Tonto telling a story about Edna requires *generative recall* — narrative-quality prose from Tonto's accumulated observations, not just a fact list.

**Verification task (Phase 0 deliverable).** Write a 50-line Python script that:

1. Seeds Tonto's representation of Edna with 10 fact-style observations drawn from the comic transcripts (Edna's appearance in Tonto's stories).
2. Asks Tonto's actor agent: "Tell me a story about Edna."
3. Evaluates the output for:
   - Coherence as narrative (not just fact recitation)
   - Tonto's voice (long sentences, robot-narrator register)
   - Witness-boundedness (no facts beyond what was seeded)

**If the verification fails** (the representation surfaces only facts, not narrative material): augment the actor invocation contract with a "memory bundle" — explicit recent past-session messages that Tonto witnessed, attached to the scene-setup. This becomes a routine pattern for any actor invocation that requires generative recall over distant memories.

**If the verification passes**: the default API surface is sufficient. Document the Tonto-storytelling pattern as the canonical example in [actor invocation](../actor-agents/invocation.md).

This verification is a Phase-0 gate — must pass before Phase 1 begins. See [Phase 0 deliverables](../../phases/phase-0-preproduction/deliverables.md).

## Cost considerations

`observe_others=true` triggers reasoning per present peer per scene. A 5-character scene = 5 representation updates. Phase 1 will instrument this and tune cap/cadence if cost becomes meaningful.

## What representations *are*

- Directional and per-pair: Othon's representation of Sans-Nom is a different object from Honorata's representation of Sans-Nom.
- Scoped to what the observer has actually witnessed.
- Updated automatically by Honcho on commit (with optional Narrator-triggered manual updates).
- Queryable per turn for actor invocation.

## What representations are *not*

- They are not character cards. Character cards (peer cards) describe who *I am*; representations describe what *I think of someone else*.
- They are not consensus state. There is no "the truth about Aghora." There are only the truths each peer holds about her.
- They are not editable by anyone except the observing peer (and the Narrator on the observer's behalf).
