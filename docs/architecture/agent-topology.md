# Agent Topology

Current Sans-Nom uses a Writer/Narrator split. The Writer plans bounded story
structure. The Narrator turns one accepted node into player-facing prose. Actor
agents and LoreExpert/Lore Authority are support roles, not the main authoring
surface.

```text
                 +-------------------------------+
                 |            PLAYER             |
                 |     choice or free text        |
                 +---------------+---------------+
                                 |
                                 v
                 +-------------------------------+
                 |          PLAY CONTROLLER       |
                 | packet follow, refresh trigger |
                 +-------+---------------+-------+
                         |               |
          needs packet   |               | selected packet node
                         v               v
          +---------------------+   +----------------------+
          |       WRITER        |   |       NARRATOR       |
          | StoryBible/Packet   |   | player-facing prose  |
          | refresh planning    |   | choices + commit     |
          +----+-----------+----+   +----------+-----------+
               |           |                   |
               |           | optional          | optional actor_needs
               v           v                   v
       +-------------+  +------------+   +----------------+
       | Scenarist   |  | LoreExpert |   | Actor agents   |
       | scene ideas |  | authority  |   | witnessed role |
       +-------------+  +------------+   +-------+--------+
                                                |
                                                v
                                      +------------------+
                                      |      HONCHO      |
                                      | scene memory and |
                                      | representations  |
                                      +------------------+
```

## Roles

**Play controller**

- Loads the current story state.
- Follows valid in-packet choice targets before calling Writer refresh.
- Triggers Writer refresh for free text, missing targets, terminal handoff, or
  exhausted packets.
- Persists displayed turns before model-heavy work so resume is safe.

**Writer**

- Owns structure: `StoryBible`, bounded `StoryPacket`, node graph, choices,
  actor needs, memory intents, and continuation notes.
- In scenario-contract modes, packages the selected lorepack's authored
  scenario contract instead of inventing a fresh premise.
- Uses scoped read tools in the active agent workspace, including staged
  `lorepack/`, `story/`, `workshop/`, and `scenarists/` material when present.
- May ask Scenarist for one focused advisory pass at a time.
- May ask LoreExpert for source-grounded fidelity checks.

**Narrator**

- Owns final prose for one selected packet node.
- Receives current canon, the active packet node, relevant lore-fidelity
  constraints, and any scoped actor output.
- Presents choices exactly from the accepted packet node.
- Prepares the scene commit and TTS material.

**Scenarist**

- Brainstorms, criticizes, stress-tests, and suggests staging.
- Does not own final story structure.
- Reads only explicitly staged workspace paths. Source-pattern grounding should
  go through LoreExpert unless the Writer has deliberately staged a `lorepack/`
  path for the current request.

**LoreExpert / Lore Authority**

- Answers focused lore and fidelity questions from the active pack.
- In scenario-contract modes, acts as a reference authority rather than an
  "influence-only" helper.
- Helps identify contradictions, missing pressures, and choices that do not
  matter inside the lore contract.

**Actor agents**

- Optional scene participants invoked only when the accepted packet node has
  `actor_needs` and the referenced actor has a valid card.
- Receive their own card and witness-bound Honcho representations.
- Return raw performance material. The Narrator decides what surfaces.
- Are not used for the first seed runway unless the packet explicitly has
  supported actor needs.

**Honcho**

- Receives only rolled-out canon: player decision, final Narrator prose,
  surfaced actions/speech, scene metadata, and witness lists.
- Does not receive Writer drafts, unused packet branches, Scenarist notes, or
  actor private interiors as shared scene memory.
- Uses scene-scoped sessions so only characters present in a scene observe that
  scene.

## Admin And Observability

The admin "Behind The Scenes" view normalizes model calls, tool calls, provider
metadata, prompts, responses, and errors into an office-chat transcript. Raw
JSONL/detail logs remain the forensic source of truth.

See also:

- [Writer / Narrator split](writer-narrator-split.md)
- [Agent filesystem](agent-filesystem.md)
- [Honcho sessions](honcho-integration/sessions.md)
- [Actor invocation](actor-agents/invocation.md)
