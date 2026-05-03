# Narrator Authoring Loop

A single turn of the game, end to end. This is the engineering side of the [core loop](../../mechanics/core-loop.md).

## The loop

```
1. Player submits: "I ask my mother where my father is."

2. Narrator agent activated.
   Context loaded:
     - Narrator's own peer card (lore-passion traits)
     - Current scene state: location, time, present actors
     - Honcho representation of player from "narrator" target
       (what the Narrator has been tracking about Sans-Nom's evolution)
     - Recent scene history (last ~3 scenes verbatim, earlier summarized)
     - Retrieved canonical lore relevant to "father" question
       (vector search over the user's character lore + relevant comic
       page transcripts)

3. Narrator decides which actors are needed:
   For this scene: Aghora is present (Sans-Nom's parent),
                   Sans-Nom is present (the player).
                   Tonto is ambient (could be addressed but not required).

4. Narrator queries Aghora's actor agent:
   Provided to Aghora:
     - Aghora's lore card (the user's existing character file)
     - Aghora's representation of Sans-Nom (Honcho directional, target=sans-nom)
     - Aghora's representation of Steelhead (relevant to "father" question)
     - The scene-setup prompt: "Sans-Nom, age 8, just asked you where his father is.
       This is the first time he's asked. You have not yet decided what to tell him.
       Output your interior reaction (3 sentences) and what you say aloud (1-3 sentences)."

5. Aghora's actor returns:
   Interior: [grief-paralysis triggered; she sees Steelhead's stasis pod in her
   peripheral vision of the room next door; she has lied about this for eight
   years and the lie is starting to cost her.]
   Dialogue: "Your father is sealed away. That is all I will tell you today."

6. Narrator synthesizes the scene:
   Writes prose that includes Aghora's response, Sans-Nom's reaction
   (drawing on the player's character-arc evolution), the room's
   atmosphere, Tonto's robotic body language in the corner. Decides
   the moment lands hard. Lengthens the scene. Ends with three choice
   options + free-text.

7. Narrator commits scene to Honcho:
   - Aghora witnessed: this exchange. Updates Aghora's representation of Sans-Nom.
   - Sans-Nom witnessed: Aghora's verbal response only (not her interior).
     Updates Sans-Nom's representation of Aghora ("she has secrets about my father").
   - Tonto witnessed (ambient): updates Tonto's awareness.
   - Stasis pod is now a known-to-player concept; flagged in Honcho.

8. Narrator returns prose + choices to player.
```

This is the whole loop. The architecture's elegance is that **everything that makes this game emotionally specific is happening in step 4 (actor query) and step 7 (Honcho update)**. The Narrator is the puppeteer; the actors are the puppets-with-souls; Honcho is the thread that makes a puppet remember.

## In Go (pseudocode)

```go
// One turn of the game.
func (g *Game) Turn(ctx context.Context, input PlayerInput) (Scene, error) {
    // 1. Assemble Narrator context.
    bundle, err := g.bridge.SceneContext(ctx, SceneContextRequest{
        SessionID:        g.currentSession,
        PlayerPeerID:     "sans-nom",
        RecentSceneCount: 3,
        IncludeReprFor:   []string{"sans-nom", "narrator"},
    })

    // 2. Lore retrieval.
    loreSnippets := g.lore.Search(ctx, input.Text, k: 5)

    // 3. Decide which actors are present.
    presentActors := g.scene.PresentActors(g.currentSession)

    // 4. Fan out: query each actor in parallel.
    actorOutputs := make(chan ActorOutput, len(presentActors))
    for _, actor := range presentActors {
        go func(a Actor) {
            repr, _ := g.bridge.GetRepresentation(ctx, a.ID, target: "sans-nom")
            output := g.actors.Query(ctx, a, ActorQuery{
                SceneSetup:   bundle.LastNarratorBeat,
                MyRepr:       repr,
                PlayerInput:  input,
            })
            actorOutputs <- output
        }(actor)
    }
    outputs := collect(actorOutputs, len(presentActors))

    // 5. Narrator synthesizes.
    scene, err := g.narrator.Compose(ctx, NarratorInput{
        Bundle:         bundle,
        LoreSnippets:   loreSnippets,
        ActorOutputs:   outputs,
        PlayerInput:    input,
    })

    // 6. Commit to Honcho.
    g.bridge.CommitScene(ctx, CommitRequest{
        SessionID:   g.currentSession,
        Scene:       scene,
        Witnesses:   presentActors,
    })

    return scene, nil
}
```

Concrete LOC estimate for the loop itself: ~400 Go lines. The character cards + lore + prompt scaffolding are existing assets; this is glue + orchestration.

## Streaming

Narrator prose streams to the player UI token-by-token; the committed-scene message goes to Honcho only after the stream completes. This keeps player-perceived latency low without complicating the Honcho commit semantics. See [ADR 0003](../decisions/0003-streaming-narrator-output.md).
