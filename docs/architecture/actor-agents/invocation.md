# Actor Invocation

How the Narrator calls an actor agent during a turn. Step 4 of the [authoring loop](../narrator-agent/authoring-loop.md).

## Invocation contract

The Narrator provides each invoked actor with:

1. **The actor's lore card** — loaded from accepted peer-card artifacts in the selected lore pack, for example `lore/packs/<pack_id>/comics/peer-cards/cards/accepted/<peer_id>.json`.
2. **The actor's directional representations** of every other character present in the scene — fetched from Honcho per-call. For a scene with Aghora and Sans-Nom present, Aghora's actor receives Aghora's representation of Sans-Nom (and Aghora's representation of Steelhead if Steelhead is contextually relevant — the Narrator decides what's relevant).
3. **The scene-setup prompt** — the Narrator's authored description of where the actor is, what just happened, and what the actor is being asked to output. Bounded format: typically 1–3 sentences of context plus a request for "interior reaction (3 sentences) + dialogue (1–3 sentences)" or similar.

## Invocation request format

```go
type ActorQuery struct {
    SceneSetup    string                     // Narrator's setup prompt
    MyRepr        []DirectionalRepresentation // observer=me, target=each other present character
    PlayerInput   PlayerInput                 // what the player just did, if relevant
}

type ActorOutput struct {
    Interior  string  // not shown to player; informs Narrator's synthesis
    Dialogue  string  // potentially surfaced verbatim, potentially reframed
    Action    string  // physical action description
}
```

## What the actor does NOT receive

- The Narrator's previous prose. (Breaks the [actor-bleed](../narrator-agent/failure-modes.md) chain.)
- Other actors' outputs. (Each actor responds independently; the Narrator is the only one who sees all outputs.)
- The player's verbatim free-text. (The Narrator passes a *summary* of the player's input as part of the scene-setup, not the raw text. This protects the actor from prompt-injection attempts disguised as character speech.)
- Lore beyond what's in its own card and its own representations. (Lore retrieval is the Narrator's job, not the actor's.)

## What the Narrator does with the output

The actor's output is *raw material*. The Narrator synthesizes — picks dialogue lines (verbatim or reframed), incorporates interior reactions into the prose's atmosphere, decides whether to surface an action as foregrounded or background, decides what *not* to show. The actor's output is never shown to the player.

This is the "director synthesizing performances" model. The actor is the actor; the Narrator is the director and the prose author.

## Cost model

- Sonnet tier per actor call.
- Default cap: 4 actors per turn (see [failure-modes.md](../narrator-agent/failure-modes.md)).
- Per-call token usage instrumented per-actor for cost analytics — see [observability.md](../observability.md).

## Concurrency

Actor calls fan out in parallel. The Narrator collects all outputs before synthesizing. See the goroutine pattern in the [authoring loop pseudocode](../narrator-agent/authoring-loop.md).
