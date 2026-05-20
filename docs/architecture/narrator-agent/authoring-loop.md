# Narrator Authoring Loop

This page describes the foreground turn loop after the Writer has produced or
refreshed a packet. For the structural planning layer, see
[Writer / Narrator split](../writer-narrator-split.md).

## Normal Turn

```text
1. Player chooses an offered option or enters free text.

2. Runtime resolves the decision:
   - if the choice target exists in the current packet, follow it;
   - otherwise trigger Writer refresh.

3. Runtime loads the selected packet node:
   - StoryBible and current canon;
   - active lorepack contract and scene constraints;
   - packet node beats, choices, actor needs, and memory intents;
   - recent committed turns.

4. Runtime stages a scoped Narrator workspace:
   - story/ for current state and continuity files;
   - lorepack/ for active pack contract/reference files;
   - scenarists/ only when relevant advisory artifacts are part of context.

5. Supported actor needs are resolved:
   - unsupported/missing actor cards are skipped or rejected with logs;
   - present actors receive only their own card, representations, and bounded
     setup.

6. Narrator writes the player-facing scene:
   - no new branch targets;
   - choices exactly match the accepted node;
   - lore pressure appears through action and consequence.

7. Runtime checkpoints the displayed turn, synthesizes TTS if enabled, and
   commits the scene to Honcho with the true witness list.
```

## Pseudocode

```go
func (g *Game) Turn(ctx context.Context, decision PlayerDecision) (TurnView, error) {
    target, needsRefresh := g.ResolveDecision(decision)
    if needsRefresh {
        packet, err := g.writer.Refresh(ctx, WriterInput{
            Canon:        g.CurrentCanon(),
            LoreContract: g.ActiveLorepackContract(),
            Decision:     decision,
        })
        if err != nil {
            return g.PendingTurnOrError(err)
        }
        target = packet.EntryNode()
    }

    node := g.CurrentPacket().Node(target)
    actorOutputs := g.ResolveActorNeeds(ctx, node.ActorNeeds)

    turn, err := g.narrator.Rollout(ctx, NarratorInput{
        Canon:        g.CurrentCanon(),
        LoreContract: g.ActiveLorepackContract(),
        Node:         node,
        ActorOutputs: actorOutputs,
    })
    if err != nil {
        return TurnView{}, err
    }

    g.story.SaveDisplayedTurn(turn)
    g.tts.SynthesizeIfEnabled(ctx, turn)
    g.bridge.CommitScene(ctx, CommitRequest{
        SessionID: g.SceneSessionID(node),
        Scene:     turn,
        Witnesses: g.WitnessesFor(node),
    })
    return turn, nil
}
```

## Streaming And Checkpointing

Player-facing prose may stream to the UI, but Honcho receives only the complete
committed scene. The runtime persists displayed-turn state before expensive or
fallible follow-up work so resume remains safe after TTS or bridge failures.

See [ADR 0003](../decisions/0003-streaming-narrator-output.md) for the
stream-then-commit decision.
