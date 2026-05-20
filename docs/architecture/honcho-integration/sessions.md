# Sessions - Implementation

For the game-design view of sessions, see
[`mechanics/memory-model/sessions.md`](../../mechanics/memory-model/sessions.md).
This file is the engineering side.

## Current Rule

Sans-Nom uses scene-scoped Honcho sessions.

The session id is derived from the story session plus the accepted packet/node
scene boundary. Practically: a narrated node is the unit committed to Honcho.
This keeps witness memory tight and avoids overloading one long session with a
large cast.

## Why Scene-Scoped Sessions

Honcho observers are session participants. If a session has too many observers,
the sidecar can reject the commit. More importantly, a character should not gain
access to events they did not live through.

Scene-scoped sessions preserve the core witness rule:

- only actors present in the current scene observe the current scene;
- if a new actor enters mid-way, that is a new scene/session boundary;
- earlier scene content remains available through committed summaries and
  directional representations, not through direct witnessed session presence.

## Commit Shape

The Go runtime tells the sidecar which session id to use per commit:

- `POST /v1/scene/context` fetches scene-local history and relevant
  representations.
- `POST /v1/scene/commit` writes the finalized scene and witness list.

Only committed Narrator text and surfaced actions/speech become scene memory.
Writer drafts, Scenarist notes, and actor private interiors are not committed as
shared witnessed canon.

## Cross-Scene Memory

Peer cards and directional representations are workspace-scoped. They persist
across scene sessions. When a character returns later, the runtime queries
Honcho for that character's current representation instead of assuming direct
access to old session transcripts.

## Operational Notes

- Keep witness lists small and true to the scene.
- Treat sidecar timeouts as recoverable when the local story checkpoint already
  captured the pending turn.
- Use `SANS_NOM_BRIDGE_TIMEOUT_MS` to raise local bridge timeout during slow
  Honcho commits.
- Log commit duration, session id, witness count, and timeout status.

## Session Deletion

Sessions are not deleted in normal play. A story/playthrough can be archived;
archived state remains available through story storage and Honcho according to
the retention policy.
