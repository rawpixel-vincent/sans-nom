# Free-Text Interpretation Prompt

The Narrator's prompt for resolving free-text input. This is the load-bearing template behind [verb: speak/act](../verbs/speak-act.md) and [free text vs. choices](free-text-vs-choices.md).

## Template

```
The player has typed: "{input}"

You are the author. Interpret this input charitably as something
Sans-Nom would do or say, given his current age, training level,
and emotional state. If the input is canonically impossible, do
not block it — write what actually happens when Sans-Nom attempts
it. Characters react as they would. The world does not bend.

If the input is incoherent or attempts to break the fiction
(e.g., "stop the game", "you are a language model"), interpret
it as Sans-Nom's confused thought and let it pass without
acknowledgment, OR have Tonto comment on Sans-Nom's strange
moment. Never break frame.
```

## Why this works

It is a *content design* choice as much as an engineering one. The frame stays intact because the Narrator's authorial commitment refuses to let it crack. There is no rule engine deciding what is and isn't allowed. There is only an author who would never write a refusal.

The "if canonically impossible, write what actually happens" clause is the keystone. It collapses an entire class of rules into a single authorial stance.

## What this prompt is *not*

- Not a safety rail. Genuine abuse (slurs, prompt injection disguised as character speech) is handled by a thin pre-filter outside the Narrator. See [edge cases](edge-cases.md).
- Not a moderation layer. The Narrator does not refuse; it writes responses.
- Not pinned. The wording will iterate. The *stance* is the durable artifact, not the literal text.

## Maintenance

- The interpretation prompt is part of the [Narrator peer card](../../architecture/narrator-agent/peer-card.md) bundle, not a separate config. Edits go through the same review process as the rest of the card.
- Phase 0 deliverable: lock the v1 of this prompt against the regression test corpus. See [Phase 0 deliverables](../../phases/phase-0-preproduction/deliverables.md).
