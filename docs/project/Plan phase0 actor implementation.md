# Master Plan: Evolving Actor Agents

## Summary
Build the full actor-memory architecture around the existing Writer/Narrator/Honcho spine. Actors become callable, witness-bound agents whose cards and directional representations evolve during play. Writer may call actors as planning material through the Codex local-tool backend; Narrator invokes actors for the selected scene and owns all curated memory updates. Actor private interiors stay out of shared canon.

## Key Architecture Changes
- Add a first-class actor material flow:
  - `ask_actor` local tool for Writer on the Codex tool backend.
  - Existing pre-Narrator actor invocation remains the selected-node rollout path.
  - Actor output is planning/material unless Narrator surfaces it as `surfaced_actor_messages`.
- Add actor card evolution:
  - Source/local actor cards are immutable seed material.
  - On first actor use, ensure the actor has a Honcho self-card seeded from the local/source card if Honcho is empty.
  - Narrator can append curated card facts after a successful scene commit.
  - Actors cannot self-update; actor output may suggest change, but Narrator decides.
- Fix witness participation:
  - Extend scene commit shape with explicit observer peer IDs, separate from broad witness labels.
  - Only real peers/registered actors become Honcho session observers; ambient groups remain metadata labels.
  - Silent present actors can evolve from witnessing a scene without being forced to speak.
- Preserve memory boundaries:
  - Writer drafts, Scenarist notes, unused branches, and actor private interiors never enter shared scene memory.
  - Surfaced actor dialogue/action enters canon only through Narrator-selected `surfaced_actor_messages`.
  - Actor-private/evolved facts go to Honcho cards, not scene commits.

## Public Interfaces And Types
- Add `actors.ActorCallRequest` and `actors.ActorMaterial`:
  - request: `actor_id`, `mode`, `scene_setup`, `question`, `targets`, optional `node_id`.
  - result: `actor_id`, `interior`, `dialogue`, `action`, `used_card_source`, `used_representations`, `artifact_path`.
- Add `ask_actor` as a Codex local tool alongside `ask_scenarist` and `ask_lore_expert`.
  - Writer prompt instructs: use it only when a character’s will, memory, voice, or relationship pressure affects packet design.
  - Tool results are planning-only and written under the Writer model-run artifacts.
- Extend `narrator.Draft` with `curated_card_updates`.
  - update: `peer_id`, optional `target_peer_id`, `facts`, `reason`.
  - Runtime applies these after successful Honcho scene commit via existing peer-card patch API.
- Extend scene commit / sidecar request with `observer_peer_ids`.
  - Sidecar adds those peers to the Honcho scene session with `observe_others=true`.
  - Existing `witnesses` remain event metadata and player-facing/audit labels.

## Implementation Plan
- Actor context core:
  - Centralize actor context assembly so Writer tool and Narrator rollout use the same card, representation, target, and fallback logic.
  - Include Honcho self-card first, local/story card fallback second, directional representations for player plus requested targets, and compact scene setup.
  - Keep Phase 0 actor cap at 4 per selected node/tool call.
- Writer actor calls:
  - Implement `ask_actor` only for Codex local-tool Writer runs first.
  - Tool builds a synthetic actor need from Writer’s request, invokes the actor, logs request/response, and returns a compact artifact path plus structured actor material.
  - Writer may use material to shape beats, choices, and `actor_needs`, but the accepted `StoryPacket` remains the only Writer output.
- Narrator actor calls:
  - Keep invocation before Narrator render, but strengthen prompts so Narrator treats actor material as raw performance, not canon.
  - Preserve current filtering: Narrator may surface only actor IDs that were actually invoked.
  - Add logs that connect actor raw output, surfaced messages, and resulting commit events.
- Memory evolution:
  - Add a card-update applier in the play loop after `CommitScene` succeeds.
  - Deduplicate appended facts, cap facts with existing card limits, and log `honcho.card_update.request/completed/failed`.
  - Store failed card updates as pending turn state for retry, similar to pending commits.
- Sidecar/Honcho:
  - Add `observer_peer_ids` handling to `bridge/src/bridge/honcho_ops.py`.
  - Normalize observer IDs with the existing `honcho_peer_id` logic.
  - Do not add ambient witness labels as observers.
  - Keep dev fallback behavior deterministic.

## Test Plan
- Unit tests:
  - `ask_actor` schema, unsupported actor rejection, artifact creation, and logging.
  - Actor context assembly prefers Honcho card, falls back to story/local card, and includes directional representations.
  - Writer prompt mentions `ask_actor` and planning-only actor material.
  - Narrator draft schema accepts `curated_card_updates`; actor interiors still never commit.
  - Card updates apply only after successful commit and are persisted for retry on failure.
  - Sidecar adds only `observer_peer_ids` as session peers, not broad witness labels.
- Integration tests:
  - Static play loop with an actor need produces actor raw material, surfaced actor message, commit event, and card update.
  - Later actor invocation sees the newly appended card fact.
  - A non-present actor does not receive observer memory from a scene.
- Acceptance scenarios:
  - Iri can be present, invoked, speak/act through actor material, and later carry an evolved self-card fact.
  - Tonto can generate witness-bound recall from his own representations.
  - Aghora-style long-play test shows stable voice and no knowledge leakage across scenes.

## Assumptions
- First direct Writer actor calls target the Codex local-tool backend.
- Claude/Sonnet keeps existing Narrator-side actor invocation until a later custom-tool or precomputed-material layer is designed.
- Narrator-curated Honcho card updates are the v1 memory authority.
- Empty `target_peer_id` means the actor’s self/evolved card; non-empty target means directional card about another peer.
- Actor private interior can inform Narrator prose but is never committed to shared scene memory.
