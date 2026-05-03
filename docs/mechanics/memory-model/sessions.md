# Sessions — Game-Side Rationale

A session is the game-design unit of *a stretch of time in one place with a stable cast*. Mechanically (in Honcho) it is a session object; narratively it is a chapter of Sans-Nom's life.

For the implementation side — API boundaries, cross-session inheritance, cost — see [`architecture/honcho-integration/sessions.md`](../../architecture/honcho-integration/sessions.md).

## What makes a session

A session is opened when:

- The location changes durably (Sans-Nom leaves the compound to travel; a session ends and a new one opens at the destination).
- A long [Wait](../verbs/wait.md) compresses time enough that the cast has shifted.

A session is *not* opened when:

- The location changes briefly (Sans-Nom walks from kitchen to courtyard within the same arc — this is a scene transition, not a session boundary).
- A new character enters or leaves a scene within the same arc.

The default vertical-slice session is "Childhood at Aghora's compound" — one session, ages 6–12, ~80 scenes. See [phase-1 slice spec](../../phases/phase-1-vertical-slice/slice-spec.md).

## Why sessions exist as game-design units

- **Memory continuity at the right granularity.** Within a session, the Narrator has working access to scene-level history. Across sessions, the Narrator queries character representations and summary state. This matches how saga storytelling actually works — chapter coherence + cross-chapter character continuity.
- **Session boundaries are narrative beats.** A session ending is a *moment* — the moment of leaving the compound, the moment a year passes. The player should feel the weight of these.
- **Session boundaries are checkpoints.** End-of-session is when the [scene-end checkpoint card update](card-updates.md) runs.

## What players should not be told

The session structure is invisible to the player. The player does not "save and quit between sessions." Sessions are an authoring unit, not a UI unit. From the player's perspective, time flows continuously and they can play across session boundaries without noticing — the Narrator handles the seam.
