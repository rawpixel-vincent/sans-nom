# Story Run Analysis: story-20260520-152942

Date: 2026-05-21

Scope: analyze the current Metabarons story run after the player reached turn 5, using local story artifacts, logs, packets, Writer/Narrator outputs, and the active Metabarons lorepack contract. The runtime has also generated and committed a pending turn 6, so this analysis covers turns 1-5 plus the generated turn 6 handoff.

## Artifacts Inspected

- Story state: `data/stories/story-20260520-152942/state.json`
- Story bible: `data/stories/story-20260520-152942/bible.json`
- Turn log: `data/stories/story-20260520-152942/turns.jsonl`
- Narration files: `data/stories/story-20260520-152942/narration/turn-000001.md` through `turn-000005.md`
- Pending displayed turn: `data/stories/story-20260520-152942/pending-turn.json`
- Packets:
  - `data/stories/story-20260520-152942/packets/packet_metabarons_spare_key_opening_001.json`
  - `data/stories/story-20260520-152942/packets/packet_metabarons_spare_key_refusal_aftershaft_002.json`
  - `data/stories/story-20260520-152942/packets/packet_metabarons_spare_key_laundry_schism_003.json`
- Writer refresh artifacts:
  - `data/stories/story-20260520-152942/model-runs/writer/turn-000003/input.txt`
  - `data/stories/story-20260520-152942/model-runs/writer/turn-000003/accepted-packet.json`
  - `data/stories/story-20260520-152942/model-runs/writer/turn-000005/input.txt`
  - `data/stories/story-20260520-152942/model-runs/writer/turn-000005/accepted-packet.json`
- Runtime log: `data/stories/story-20260520-152942/logs/events.jsonl`
- Lorepack contract: `lore/packs/metabarons/lore-pack.json`
- Staged lore reference used by agents: `data/stories/story-20260520-152942/agent/narrator/turn-000001/lorepack/wiki/lore.md`

## Current State

The current saved story is:

- Story: `The Spare Key of the Metabunker`
- Lorepack: `metabarons`
- Current packet: `packet_metabarons_spare_key_laundry_schism_003`
- Current node: `turn_6_laundry_tariff`
- Status: `awaiting_player_choice`
- Latest fully persisted turn in `turns.jsonl`: turn 5
- Displayed pending turn: turn 6, committed, with three choices

The run has not crashed in the current saved state. Honcho commits completed for each displayed turn, and story index refreshes use `replace: true`, so the stale-index fix appears active in this run.

## Turn Chain

| Turn | Packet | Node | Player input | Result |
| --- | --- | --- | --- | --- |
| 1 | opening_001 | `turn_1_intro` | cosmetic `flavor_watch_sava` | Introduces Saryn, Sava, the Witness Gallery, the severing bench, the awakened knife, and the witness-servants. |
| 2 | opening_001 | `turn_2_decision` | real choice `choice_refuse_love` selected next | Presents three strong stance choices: submit body, refuse for love, invoke memory. |
| 3 | opening_001 | `turn_3_refuse_love` | `choice_refuse_love` | Saryn breaks the witness circle, frees Sava, takes a visible collar-burn, and flees into service shafts. Terminal handoff. |
| 4 | aftershaft_002 | `turn_4_service_shaft_tally` | `choice_carry_the_mask` selected next | The escape bites: the hatch counted the servant helper, Sava reveals the hidden order is older than the current Steward, and wardens approach. |
| 5 | aftershaft_002 | `turn_5_carry_the_mask` | `choice_carry_the_mask` | Saryn unmasks the hatch-servant, preserving a living witness but making them huntable. Terminal handoff. |
| 6 | laundry_schism_003 | `turn_6_laundry_tariff` | waiting | The laundry level demands payment for bringing dynastic war below; choices spend the living memory, Saryn's body, or trust the witness to lullaby-keepers. |

The three-turn seed runway worked structurally: intro -> decision -> branch, then Writer refresh generated the next packet. The later packet also follows the intended pattern: entry choice node plus branch handoff nodes.

## Runtime Health

What looks healthy:

- Seed story index was ingested with `replace: true` at creation.
- Story index was rebuilt and re-ingested with `replace: true` after Writer refresh.
- Honcho commits were scene-scoped by packet/node session id:
  - turn 1: 8 witnesses, 3 events
  - turn 2: 8 witnesses, 3 events
  - turn 3: 7 witnesses, 2 events
  - turn 4: 7 witnesses, 3 events
  - turn 5: 6 witnesses, 2 events
  - turn 6: 8 witnesses, 3 events
- Writer refresh context includes the story bible, scenario contract, selected node summary, recent canon, and `STORY PROGRESSION CONTRACT` before the full packet reference.
- Writer refreshes produced accepted packets at turns 3 and 5.
- Scenarist usage is sequential in this run. Turn 5 has one `scenarist.ask.started` and one `scenarist.ask.completed`, not parallel answer clusters.

What looks unhealthy or at least noisy:

- Writer refresh metadata for turn 3 and turn 5 says `latest_decision: "observe"`, even though the actual branch choices were `choice_refuse_love` and `choice_carry_the_mask`.
- The Writer context does include the real choice inside `RECENT WITNESSED CANON`, but the high-priority section `LATEST ROLLOUT AND DECISION` incorrectly says `observe`.
- Narrator repeatedly uses file tools to inspect broad lorepack surfaces, including `lorepack/wiki/lore.md` and sometimes large manifests. This did not fail, but it is expensive and can pull the Narrator toward source-summary imitation instead of immediate scene narration.
- Scenarist tried to read `packets/packet_metabarons_spare_key_refusal_aftershaft_002.json` from its scoped workspace and got "File does not exist." The scoped workspace exposed `story/current-packet.json`, not durable `packets/...` paths. The Scenarist recovered by reading other files, but this is the same path-contract class of bug we have hit before.
- Writer made an `LS` command with a pipe, `find story -maxdepth 4 -type f | grep -i actor`; it returned no files. It did not break the run, but it shows the agent still expects actor-card material to exist in a path that is not staged.

## Story Feel

The opening is much stronger than the older broad-invention seeds. It has a playable immediate pressure, a visible player desire, and choices that express stance. The first meaningful decision is not navigation; it is a moral declaration under witness. That part is working.

The prose has real material force. The Witness Gallery, milk-cleaned severing bench, memory glass, oil, masks, service hatch, collar burn, and laundry ash ledgers all feel tactile. The story is not generic space fantasy. It has a coherent pressure field: body, witness, weapon, service, inheritance, consequence.

The emotional core is also clearer than before: Saryn wants Sava alive and does not want a loved person reduced to proof-material. That gives the choices a human axis.

The weakness is saturation. The story keeps inventing new objects of record and proof: witness masks, memory glass, tally teeth, debtor tags, broken masks, breath-memory, altered lullabies, ash drawers, wet ledger-cloths. They are individually good, but together they make every turn feel like another local bureaucracy of memory. The danger is not that nothing happens. Things do happen. The danger is that the same kind of thing keeps happening: proof is made fragile, servants react, memory must be carried, another tariff is named.

The run has advanced locations:

- Witness Gallery
- service hatch / service shafts
- witness sleeping niches
- linen chute
- laundry level

But the dramatic center has mostly stayed on one axis: "Can Saryn preserve servant-carried testimony about the broken witness rite?" That is a strong act-one pressure, but by turn 6 it needs to become a bridge into the next macro phase: weapon claim, rival claimant, family figure, or a concrete older-order author. If it stays inside servant proof economy for another packet, it will feel like a beautiful loop.

The current pending turn 6 choices are better than navigation, but still clustered:

- spread the mask-memory into many mouths
- mark Saryn into the ash ledger
- send the witness to the lullaby-keepers and go to ash drawers

All three are variations on servant memory management. They change cost and risk, but they do not yet introduce a new antagonist, weapon, family relation, or external dynastic consequence. This is the precise moment where the story should turn outward.

## Relationship To The Metabarons Source

The run is faithful to the current scenario contract:

- Family, body, weapon, memory, and duty are inseparable.
- Power requires visible cost.
- Refusal has public cost.
- Servants and witnesses shape legitimacy.
- Violence is framed as argument, not only spectacle.

It also maps clearly to source patterns in the Metabarons lore:

- Metabaronic initiation: source lore emphasizes mutilation and prosthetic transformation as initiation proof. Saryn's severing bench, inheritance jaw, collar-burn, and body-as-proof logic are faithful adaptations of that pressure.
- Castaka legitimacy: the hidden Castaka mark and lineage transfer echo the source's Castaka crest/birthmark inheritance.
- Metabunker as household-war machine: the story uses the Metabunker as fortress, vault, household, memory system, and family tomb. That is a good fit with the source's recurring Metabunker frame.
- Servant witness pressure: the source has important servant and narrator figures, especially Tonto/Lothar and Iku-Itta. This run pushes servant memory into a central mechanic, which is a valid extrapolation.
- Weapon lineage: the awakened maintenance knife and key-blade setup are close to the source's pattern where weapons and body modifications are not neutral tools.
- Operatic bodily consequence: the prose has the baroque, ceremonial, body-law pressure the pack is trying to preserve.

Where the relation is weaker:

- This is not yet "play the Metabarons story." It is an original apocryphal pocket scenario inside Metabaron-like constraints.
- The current lorepack scenario contract is broad and archetypal. It does not name a concrete canon episode, canon protagonist, canon antagonist, or volume-level story line to adapt.
- Because the contract is broad, the Writer is free to keep inventing new household sub-systems. That creates faithful texture but not necessarily faithful plot.
- Source Metabarons often escalate from household bodily law into galaxy-scale or lineage-scale catastrophe. This run has not yet opened that larger pressure. It remains in close-quarters servant testimony.
- Source Metabarons has brutal family figures and named dynastic opponents. This run has Sava and the Initiation Steward, but by turn 6 still lacks a rival claimant, parent figure, or visible family authority with personal force.

In short: the run is contract-faithful, but not yet canon-story-faithful. It honors the Metabarons flavor and core pressure well. It does not yet feel like a playable adaptation of a specific Metabarons story arc.

## Specific Issues

### 1. Writer decision context is wrong at terminal continuation

Evidence:

- `model-runs/writer/turn-000003/metadata.json` has `latest_decision: "observe"`.
- `model-runs/writer/turn-000005/metadata.json` has `latest_decision: "observe"`.
- Both corresponding `input.txt` files show:
  - `## LATEST ROLLOUT AND DECISION`
  - `observe`
- The real choices only appear later in `RECENT WITNESSED CANON`.

Impact:

The Writer can still infer the selected branch from recent canon and selected node id, but the top-level decision signal is lying. This weakens branch continuity and makes it easier for the Writer to plan generic aftermath instead of carrying the player's actual stance forward.

Likely cause:

In the TUI submit path, terminal handoff continuation with empty Enter passes an empty `nextDecision` into `refreshPacket`. The state already knows the last real decision, but `refreshPacket` receives an empty decision and renders `observe`.

### 2. The story risks servant-memory recursion

Evidence:

- Term frequency across narration plus pending turn shows high motif concentration:
  - `servant`: 64
  - `Sava`: 52
  - `witness`: 40
  - `laundry`: 32
  - `mask`: 29
  - `hatch`: 28
  - `memory`: 23
  - `ash`: 22
- Lower-frequency terms that should soon become macro-pressure:
  - `weapon`: 6
  - `body`: 5
  - `lineage`: 1
  - `Castaka`: 2
  - `Metabunker`: 3

Impact:

The story is deeply textured, but the texture is beginning to dominate plot movement. If the next Writer refresh keeps producing memory-ledger sub-systems, the player will feel like each choice pays a local tariff rather than moving toward inheritance, weapon claim, rival pressure, or family revelation.

### 3. The current lorepack scenario is still too abstract for canon-faithful play

Evidence:

The active `lore-pack.json` contract describes a "contested figure" and "household crisis" rather than a concrete scenario such as Othon's initiation fallout, Aghnar's trial, Aghora's body/lineage crisis, or No-Name's patricide.

Impact:

This gives the Writer enough constraint to avoid generic fantasy, but not enough story spine to guarantee canon-shaped progression. The Writer can produce faithful-feeling invented material indefinitely.

### 4. Narrator and Writer can over-read broad lore surfaces

Evidence:

- Narrator turn 1 reads `lorepack/wiki/lore.md`, `lorepack/comics/manifest.json`, and `lorepack/index/documents.jsonl`.
- Narrator turn 6 again reads `lorepack/wiki/lore.md`.
- Writer turn 5 reads `lorepack/wiki/lore.md`.

Impact:

The agents have access to useful lore, but broad reads are blunt. They can bloat prompt context, create preview truncation, and encourage the agent to imitate source-summary motifs instead of using retrieved targeted references. This is not a crash bug, but it is a fidelity/noise bug.

### 5. Scenarist relevant path staging is inconsistent

Evidence:

Turn 5 Scenarist attempted to read:

`data/stories/story-20260520-152942/agent/writer/turn-000005/packets/packet_metabarons_spare_key_refusal_aftershaft_002.json`

The file did not exist in its scope. The staged equivalent was:

`story/current-packet.json`

Impact:

The Scenarist recovered, but this weakens advisory quality and can reintroduce the "wrong path in instructions" problem. The tool contract should only expose workspace-relative staged paths.

## What Is Working

- The three-turn seed runway is structurally working.
- Writer/Narrator split is working: Writer packages packets, Narrator produces player-facing prose, and Honcho receives only witnessed canon.
- Scene-scoped Honcho sessions are working and staying under the observer cap.
- Lorepack scenario contract is present in Writer and Narrator contexts.
- The new story index replacement behavior is active.
- The story has much stronger immediate player pressure than the old original-germ workshop pipeline.
- Choices are meaningfully moral and material; they are not just "go left/go right."

## What Needs The Next Iteration

The next iteration should not throw away this run. It is one of the better outputs so far. The fix is to add stronger phase control and concrete canon scenario spine.

The story needs a hard progression rule after a servant-memory choice:

- One more servant consequence is fine.
- Then the next packet must force a weapon claim, rival claimant, family authority, or source-specific dynastic revelation.

The system needs to distinguish:

- contract-faithful original episode
- canon-faithful adaptation of a specific lorepack story line

Right now it says "faithful canon prototype" but behaves more like "original episode under a faithful pressure contract."
