# Cast

Each named character is a Honcho peer with its own card, its own actor agent, and its own directional representations of every other character it has witnessed.

| Peer | Source | Card seed | Active in |
|---|---|---|---|
| `sans-nom` | The player | [Seed](../player-character/sans-nom-card-seed.md) | All phases |
| `aghora` | [Council of Castaka card](../../../external/references/council%20of%20castaka/metabarons/Aghora.md) | Verbatim | Phase 1+ |
| `tonto` | New (this project) | See [tonto.md](tonto.md) — robot narrator, lore-conduit | Phase 1+ |
| `lothar` | New (Tonto's robotic companion) | See [lothar.md](lothar.md) | Phase 1+ |
| `steelhead` | [TeteDacier card](../../../external/references/council%20of%20castaka/metabarons/TeteDacier.md) | Verbatim, with phase flags (early / Melmoth / late) | Phase 1 (via stasis pod), Phase 3 (live encounter possible) |
| `vicenta` | [DonaVicenta card](../../../external/references/council%20of%20castaka/metabarons/DonaVicentaGabrielaDeRokha.md) | Verbatim, flagged as not present | Phase 1 (via Tonto's stories, recordings) |
| `aghnar` | [Aghnar card](../../../external/references/council%20of%20castaka/metabarons/AghnarVonSalza.md) | Verbatim, flagged as not present | Phase 1 (via Tonto), Phase 3 (psychic/apparitional) |
| `oda` | [Oda card](../../../external/references/council%20of%20castaka/metabarons/Oda.md) | Verbatim, flagged as not present | Phase 1 (via Tonto's stories) |
| `othon` | [OthonVonSalza card](../../../external/references/council%20of%20castaka/metabarons/OthonVonSalza.md) | Verbatim, flagged as not present | Phase 1 (via Tonto's stories) |
| `honorata` | [Honorata card](../../../external/references/council%20of%20castaka/metabarons/Honorata.md) | Verbatim, flagged as not present | Phase 1 (portraits, recordings), Phase 2+ (via Sans-Nom's training and confrontations) |
| `edna` | New ([stub](edna.md)) | TBD — designer expansion in Phase 1+ | Phase 1 (Tonto stories), Phase 3 (chest-crest if expanded) |
| Generated NPCs | [generated-npcs.md](generated-npcs.md) | Narrator on first appearance, persisted thereafter | Phase 2+ |

## What "flagged as not present" means

Most of the dynasty is dead before Sans-Nom is born. The cards exist in Honcho so that the architecture *knows* these characters and can reference their representations of each other. They are not callable as live actors in early scenes. They participate via:

- **Tonto's stories** (Tonto narrates from his witness representations of them — see [tonto.md](tonto.md))
- **Memory fragments** (recordings, holograms, sealed rooms — Narrator-summoned)
- **The chest-crest** (Phase 3 — see [dead-and-absent.md](dead-and-absent.md))
- **Apparitions** (psychic / spiritual — Aghnar specifically, Phase 3)

## Character file conventions

Each character file in this directory contains:

1. **Canonical role** — one line on who they are in the saga.
2. **Source** — link to the lore card.
3. **Game role** — when they appear, how, why.
4. **Witness scope** — what they have seen across the saga (the seed for their representations of others).
5. **Narrator notes** — what makes this character hard to write well, what canonical no-nos to avoid.

The file is *not* a copy of the lore card. It is a game-side overlay.
