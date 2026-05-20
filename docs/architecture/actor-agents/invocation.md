# Actor Invocation

Actor invocation is optional support for a selected packet node. The Writer may
request actor needs, but the runtime should invoke only actors that are present,
supported by cards, and relevant to the scene.

## Invocation Contract

Each invoked actor receives:

1. The actor's accepted card.
2. The actor's Honcho directional representations for present/relevant peers.
3. A bounded scene setup from the Narrator.
4. A summary of the player decision if relevant.

The actor returns raw performance material:

```go
type ActorOutput struct {
    Interior string
    Dialogue string
    Action   string
}
```

The Narrator decides what, if anything, appears in player-facing prose.

## What The Actor Does Not Receive

- Other actors' outputs.
- Writer drafts or unused branches.
- Scenarist notes unless the Narrator deliberately summarizes them.
- Raw player prompt-injection text.
- Repo-root lore or old story data.

## Guardrails

- Missing actor card: skip/reject the actor need and log it.
- New actor enters mid-scene: start a new scene/session.
- Actor private interior: do not commit as shared witnessed canon.
- Actor calls are not required for the first seed runway.

## Concurrency

Supported actors in the same scene can still fan out in parallel because each
actor answers independently from its own witnessed representation. This is
different from Scenarist work, which is sequential because creative advisory
passes often depend on prior answers.
