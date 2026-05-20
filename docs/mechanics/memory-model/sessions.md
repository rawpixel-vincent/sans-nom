# Sessions - Game-Side Rationale

A session is the witness boundary for a scene: who was present, what they could
observe, and which memories Honcho may update from that scene.

For the implementation side, see
[`architecture/honcho-integration/sessions.md`](../../architecture/honcho-integration/sessions.md).

## What Makes A Session

A new session opens when the witnessed scene changes:

- the player moves into a materially different scene;
- the cast changes in a way that would give a new character access to prior
  events they did not witness;
- a long wait or time compression changes who is present;
- the current packet/node changes to a new committed scene.

A session is not a player-facing save slot. It is an authoring and memory
boundary.

## Why This Matters

The game is built on witness-bound knowledge. If an actor joins a scene late,
that actor should not automatically know what happened before they arrived. If
the system keeps one huge location-arc session open, the memory substrate starts
to imply false witnesses.

Scene-scoped sessions keep the fiction honest:

- present characters observe;
- absent characters do not;
- later recall comes through representations, summaries, artifacts, or explicit
  in-fiction testimony.

## Player Experience

The player should not see session boundaries. They see prose, choices, free
text, and time passing. The runtime handles the session seams quietly so memory
remains coherent underneath.
