# Decisions Log (Cross-Cutting)

Open questions and decisions that span teams or don't fit cleanly into a single subtree. Architectural decisions live in [`architecture/decisions/`](../architecture/decisions/) as ADRs; this file is for the rest.

## Open

### 1. Narrator persona persistence across playthroughs

Does the Narrator have its own evolving peer card across all players' games, or is each playthrough a fresh Narrator?

**Recommendation.** Fresh per playthrough. Reasoning: privacy (one player's prose shouldn't bleed into another's), tone (the Narrator should not develop habits across players that weren't earned in any one player's game). But worth deciding explicitly.

**Decision deadline.** End of Phase 0.

### 2. Narrator commentary track / fourth-wall asides

The Council of Castaka has the `[Tonto, aside]` pattern. The game could surface that for the Narrator too — meta-commentary on what the Narrator is finding interesting in this player's story.

**Recommendation.** Risky (breaks frame); potentially the most interesting feature. Phase-2+ experiment, not Phase-1 commitment.

**Decision deadline.** End of Phase 1 (decide whether to design for Phase 2).

**Phase 1 research task (narrative designer).** The specific question to answer before the deadline: *Can the Council of Castaka's `[Tonto, aside]` pattern be adapted for the Narrator without breaking the fourth wall?* The CoC source is at `external/references/council of castaka/metabarons/invokation-with-peer-pressure.md`. The answer should be a one-page design note deposited in `docs/project/` before Phase 1 closes.

### 3. Workspace-per-playthrough archival

Death archives the workspace. After a year, an archived workspace is still readable via Recall — but does it remain in Honcho hosted (cost) or get exported to cold storage (latency)?

**Recommendation.** Phase 1 keeps everything live in Honcho. Phase 3 evaluates cold-storage migration based on actual archive volume.

**Decision deadline.** End of Phase 2.

### 4. Image-generation style: Gimenez-faithful or original

Faithful is more evocative for fans, riskier for licensing. Original (a new painterly style developed for the game) is safer and gives the project its own identity.

**Recommendation.** Probably original, with Gimenez as one reference among several. Aligns with [Track A/B/C licensing](risks/license.md) considerations.

**Decision deadline.** Phase 2 image-gen pipeline kickoff.

### 5. Save & resume across devices verification

Honcho's hosted backend probably handles this naturally via account-bound workspaces. Verify in Phase 1 (it's a [Phase 1 exit criterion](../phases/phase-1-vertical-slice/exit-criteria.md) implicitly).

**Status.** Verified in Phase 1.

### 6. Multiplayer Sans-Noms

Could multiple players play different Sans-Noms in the same Honcho workspace and have their stories know about each other? Two Sans-Noms in different timelines, or two characters in one timeline?

**Recommendation.** Out of scope for v1.0. Architecturally interesting; the directional-representation system was literally designed for this. Post-v1.0 candidate.

**Decision deadline.** Post-v1.0.

## Closed

(Items move here as they are decided. Each closed item links to where the decision lives — typically an ADR or a section of a doc.)

- **Stack.** → [ADR 0001](../architecture/decisions/0001-go-vs-typescript.md). Go primary + Python sidecar.
- **Honcho hosting.** → [ADR 0002](../architecture/decisions/0002-honcho-hosted-vs-self.md). Hosted in Phase 1+2; self-host as Phase 3 escape hatch.
- **Streaming Narrator output.** → [ADR 0003](../architecture/decisions/0003-streaming-narrator-output.md). Stream to UI; commit committed-text post-stream. Replay determinism = no for normal play.
- **Card eviction policy.** → [ADR 0004](../architecture/decisions/0004-card-eviction-policy.md). Narrator-curated with most-recent fallback.
- **Save model: workspace per playthrough.** → [`mechanics/player-character/multiple-sans-noms.md`](../mechanics/player-character/multiple-sans-noms.md). Each playthrough a separate Honcho workspace.
- **Free-text edge case test plan.** → [`mechanics/input/edge-cases.md`](../mechanics/input/edge-cases.md). 20 adversarial inputs in the regression corpus.
- **Lore corpus ingestion plan.** → [`architecture/lore-retrieval.md`](../architecture/lore-retrieval.md). OCR via Claude vision, manual review, embed.
- **License risk mitigation.** → [`project/risks/license.md`](risks/license.md). Three tracks defined.
- **Dev-loop ergonomics.** → [`architecture/narrator-agent/dev-loop-ergonomics.md`](../architecture/narrator-agent/dev-loop-ergonomics.md). Hot-reload of `lore/` files; CLI testbed.
- **Tonto-of-Edna verification (Phase 0 task).** → [`architecture/honcho-integration/representations.md`](../architecture/honcho-integration/representations.md). 50-line script as Phase 0 deliverable.
- **Schema for committed scene text.** → Phase 0 implementation; structured-with-prose-as-a-field. Opens the door to scene-level annotations later.
- **Narrator-as-peer.** → [`architecture/honcho-integration/peers.md`](../architecture/honcho-integration/peers.md). Yes in Phase 1; gives audit pass capability for free.
- **Death-screen design.** → [`mechanics/player-character/death-state.md`](../mechanics/player-character/death-state.md). Tonto eulogy + workspace archive naming + "Begin again" screen.
- **Lore-faithful engine pivot.** -> [`project/lore-faithful-engine-master-plan.md`](lore-faithful-engine-master-plan.md). Lorepacks become authored gameplay scenario contracts, with explicit modes for direct canon adaptation, canon-internal original episodes, and future homage-original play. Active lorepack files are readable by model tools only through scoped agent workspaces as `lorepack/`, not through repo-root `lore/packs/...` paths.
- **Agent filesystem reference.** -> [`architecture/agent-filesystem.md`](../architecture/agent-filesystem.md). Sans-Nom keeps its in-repo scoped OS workspace layer now. Turso AgentFS is a future audit/snapshot spike, not a replacement for the current path policy.
