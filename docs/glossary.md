# Glossary

IP terms (Metabarons saga) and project terms (Honcho, architecture). Cross-link from first use in each doc.

## IP terms

### Bushitaka
The sacred warrior code of the Metabarons. A path of training, discipline, and ritual obligation. Most centrally: the law that *each Metabaron must kill the prior Metabaron in single combat* in order to claim the title. The dynasty's defining cruelty and grandeur. See [bushitaka.md](mechanics/world/bushitaka.md).

### Castaka
The dynasty's clan name. The Council of Castaka (Vinni's prior project; mirrored at `external/references/council of castaka/`) is named for it. The Castaka crest is the visual emblem of the line.

### Chest-crest
The crest Sans-Nom inherits in late game (Phase 3). The architectural mechanism through which the dead speak — ancestor peers' representations of Sans-Nom become readable to him through the crest. See [dead and absent](mechanics/cast/dead-and-absent.md).

### Metabaron
The title each successive bearer of the line claims by performing the Bushitaka kill. Eight canonical Metabarons exist: Othon, Aghnar, Steelhead, Aghora — and the implied Sans-Nom (canonical successor in the comics; the player here is *a* Sans-Nom, possibly very different from the canonical one).

### Père-Mère
Aghora's identity — a being with a female body and a male brain, raised to present as male, who self-inseminated to give birth to Sans-Nom. The most paradoxical of the Metabarons. See [aghora](mechanics/cast/aghora.md).

### Refraction / refracted knowledge
The IP's structural concept: every Metabaron carries refracted understanding of every other — they see each other through the wounds of their relationships, not as anyone objectively is. The architectural feature ([directional representations](architecture/honcho-integration/representations.md)) operationalizes this.

### Sans-Nom
"Without a name" (French). The player character. Aghora's child, raised in the Bushitaka tradition. Called Sans-Nom because no one has named him; the *why* is one of the game's load-bearing questions. See [sans-nom card seed](mechanics/player-character/sans-nom-card-seed.md). Also a canonical Metabaron name in the comics; the player's Sans-Nom may or may not become that adult.

### Shabda-Oud
Honorata's order. Possessors of psychic conditioning and surgical genius. The dynasty's existence depends on Shabda-Oud knowledge (Honorata's conception of Aghnar via surgery; her conditioning of his mind). Bounded to the priestess line. See [honorata](mechanics/cast/honorata.md) and [IP commitments](mechanics/world/ip-commitments.md).

### Steelhead / Tête d'Acier
The third Metabaron. Aghora's father. Driven mad by the death of his beloved Vicenta; preserved her body; fell into the Melmoth's influence; lost his mind in stages; was finally sealed in stasis by Aghora when she could not kill him. See [steelhead](mechanics/cast/steelhead.md).

### Tonto
The dynasty's robotic narrator. Has been present for every Metabaron from Othon onward. Cannot lie about canon. The lore-conduit in this game; the [Ask Tonto verb](mechanics/verbs/ask-tonto.md) is always available. See [tonto](mechanics/cast/tonto.md).

### Other named entities

- **Lothar** — Tonto's smaller robotic companion. See [lothar](mechanics/cast/lothar.md).
- **Berard** — Vicenta's home world. The Berard noblewoman lineage.
- **Flos** — the garden planet, referenced in Aghora's interior life and possibly in scenes Sans-Nom visits or hears about.
- **Melmoth** — the entity / influence that drove Steelhead's descent into madness.
- **Halerce** — Imperial / older-order remnants the Narrator may invoke in Phase 2.
- **Edna** — a long-dead Metabaron ancestor, used in this docs as the canonical example for [Tonto-of-Edna verification](architecture/honcho-integration/representations.md).

## Project / architecture terms

### Card (peer card)
A character's profile in Honcho. Contains TRAIT and INSTRUCTION entries describing who that character is. Per-peer; per-pair when *directional*. Cap: 40 facts per card. See [card seeding](architecture/actor-agents/card-seeding.md), [card updates](mechanics/memory-model/card-updates.md), [card eviction](mechanics/memory-model/card-eviction.md).

### Directional card
A peer card scoped to a specific *target* peer. `aghora.set_card([...], target='sans-nom')` is Aghora's view of Sans-Nom. Different from her view of any other character. See [representations](architecture/honcho-integration/representations.md).

### Dreaming
Honcho's background pipeline that surfaces stable patterns from message history and proposes card updates. See [dreaming](architecture/honcho-integration/dreaming.md).

### Peer
Honcho's primitive for a character (player, actor, generated NPC, or Narrator-as-peer). One peer per character. See [peers](architecture/honcho-integration/peers.md).

### Representation (repr)
What one peer holds about another. Generated/maintained by Honcho when `observe_others=true` is set on a session. Used to inform actor invocations. See [representations](architecture/honcho-integration/representations.md).

### Session
A witnessed scene boundary. Mechanically a Honcho session; narratively invisible to the player. If a new actor enters and should not know prior events, a new scene/session opens. See [sessions (game-side)](mechanics/memory-model/sessions.md), [sessions (impl)](architecture/honcho-integration/sessions.md).

### Workspace
Honcho's top-level container. One per playthrough. Holds all peers, all sessions, all cards for that boy. See [multiple Sans-Noms](mechanics/player-character/multiple-sans-noms.md).

### Other

- **Actor agent** — a Sonnet-tier agent invoked by the Narrator to voice a character. See [invocation](architecture/actor-agents/invocation.md).
- **Agent workspace** — a curated filesystem root staged for one model role/run. Model tools read `story/`, `scenarists/`, `workshop/`, and `lorepack/` there instead of repo-root files. See [agent filesystem](architecture/agent-filesystem.md).
- **Audit pass** — the cheap-model agent that reads the last N scenes and flags drift. See [failure modes](architecture/narrator-agent/failure-modes.md).
- **Generated NPC** — character invented by the Narrator during play; persisted in the workspace. See [generated NPCs](mechanics/cast/generated-npcs.md).
- **Narrator** — the prose agent that rolls out one accepted packet node into player-facing text and choices. See [agent topology](architecture/agent-topology.md), [Narrator peer card](architecture/narrator-agent/peer-card.md).
- **Peer card** — see "Card" above.
- **Pre-filter** — the rule-based filter that catches obvious abuse and prompt-injection patterns before reaching the Narrator. See [edge cases](mechanics/input/edge-cases.md).
- **Regression corpus** — the ~20 hand-authored test scenes used as the eval set for Narrator card iterations. See [Phase 0 deliverables](phases/phase-0-preproduction/deliverables.md).
- **Sidecar** — the small Python FastAPI service wrapping the Honcho SDK. See [python-sidecar](architecture/python-sidecar.md).
