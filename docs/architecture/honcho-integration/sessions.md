# Sessions — Implementation

For the game-design view of sessions, see [`mechanics/memory-model/sessions.md`](../../mechanics/memory-model/sessions.md). This file is the engineering side.

## Why sessions matter for Honcho

Honcho's reasoning pipeline runs over message history within a session. Putting an entire playthrough in one session is wrong — the reasoning quality degrades as message count grows, and the per-call cost balloons. The right granularity is one session per location-arc.

A session is roughly the unit of cost. Boundaries should be chosen with that in mind.

## Session boundary triggers

Concrete predicate (specified in Phase 0):

```
A new session opens when ANY of the following holds:
  1. The location changes durably (Sans-Nom leaves the compound for a different planet/setting).
  2. A long Wait compresses time enough that the cast has shifted (e.g., a year passes; characters who were children are now adolescents; characters present have changed).
  3. The current session's message count exceeds 200 AND a natural beat has just ended.
```

Trigger 3 is the safety net for sessions that grow longer than expected without an obvious narrative boundary. A "natural beat" is Narrator-detected (end-of-scene with a sense of pause).

## Cross-session memory

Peer cards are workspace-scoped, not session-scoped — they persist across all sessions in the workspace. This is what enables Aghora-in-session-2 to remember Sans-Nom from session-1 (her *card* remembers; the messages of session-1 are summarized for context).

When a character returns later in the story, the Narrator queries that character's overall representation (no `target` constraint) to load their most current state. Recent-session message history is loaded as summary context; verbatim message access is bounded to the current session.

## Tonto-as-storyteller across sessions

Tonto's representations of long-dead characters span all sessions — they are stored on his peer card and on his directional cards-for-other-peers. When Tonto tells a story in session N, his actor agent reads its full historical representation, not just the current session. This is automatic in Honcho's default behavior. See [representations.md](representations.md) for the verification of generative recall depth.

## Session API

The [Python sidecar](../python-sidecar.md) exposes:

- `POST /v1/scene/context { session_id, ... }` — fetches recent scene history + relevant representations as a context bundle.
- `POST /v1/scene/commit { session_id, scene, witnesses }` — writes scene messages, triggers directional updates.

Session creation and switching are handled at the Go side; the sidecar is told which session_id to operate on per request.

## Session deletion

Sessions are not deleted in normal play. A workspace can be archived (player marks the boy as "concluded"); archived workspaces' sessions remain queryable via [Recall](../../mechanics/verbs/recall.md).

## Cost per session

Honcho's reasoning pipeline cost is roughly per-message. A 200-scene session running ~3 reasoning passes per scene is the order-of-magnitude budget unit. See [per-turn cost model](../../project/cost/per-turn-model.md) for the full math.
