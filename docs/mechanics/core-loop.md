# Core Loop

```
[OBSERVE]   →   [CHOOSE or WRITE]   →   [NARRATOR resolves]   →   [HONCHO updates]
    ↑                                                                     |
    └──────────────  scene transitions, time skips  ←─────────────────────┘
```

## The four steps

- **Observe** — read the Narrator's prose. Scenes can run from 2 paragraphs to 2 pages. The Narrator self-paces.
- **Choose or write** — the Narrator presents 2–4 choice options (the *gamebook* spine) AND a free-text input. Most players will choose; some will write. Both are first-class.
- **Narrator resolves** — the Narrator queries the relevant actor agents privately, decides what happens, and writes the next scene. The actors' raw outputs never appear to the player; the Narrator reframes everything.
- **Honcho updates** — the scene is committed to Honcho. Each character present updates their directional representation of the player and of any other characters they witnessed. Character peer cards may be updated if the scene introduced stable new facts.

## Why this loop is the whole game

Every emotional beat the game can produce is a function of this loop. The Narrator's authoring discipline ([Pillar 3](../principles/pillars/03-narrator-as-author.md)) and the witness-bound representations ([Pillar 2](../principles/pillars/02-characters-witness-bound.md)) do all the work. There are no parallel systems (combat, inventory, skill trees) competing for the player's attention.

The loop's elegance hides its difficulty. The hard work is invisible — it lives in the Narrator's [authoring loop](../architecture/narrator-agent/authoring-loop.md) and in Honcho's [directional representations](../architecture/honcho-integration/representations.md). The player sees only prose and choice options.

## Player verbs

See [`verbs/`](verbs/) for one file per verb. Verb count is deliberately small. Richness lives in *what the Narrator does with each verb*, not in verb count.

| Verb | When | Notes |
|---|---|---|
| [Read](verbs/read.md) | Default state | Most-used verb |
| [Choose](verbs/choose.md) | Click an offered option | Choices are written per-scene, never templated |
| [Speak / Act](verbs/speak-act.md) | Type into the input | Bounded by world's plausibility |
| [Ask Tonto](verbs/ask-tonto.md) | Always | Lore conduit, costs in-fiction time |
| [Reflect](verbs/reflect.md) | Always | Surfaces Sans-Nom's interior |
| [Recall](verbs/recall.md) | Always | Re-read prior scenes |
| [Wait](verbs/wait.md) | Always | Time skip, Narrator-paced |
