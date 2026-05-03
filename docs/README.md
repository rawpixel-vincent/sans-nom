# Sans-Nom — Game Design Documentation

Live spec for **sans-nom**, an LLM-driven narrative game adapting the Metabarons saga.

> *You are the boy who has not yet decided what kind of warrior he will become. The dynasty is still being written. The Bushitaka is watching.*

This `docs/` tree supersedes the monolithic [`initial-draft.md`](../initial-draft.md), kept at repo root as historical archive. See [`glossary.md`](glossary.md) for IP terms.

## Pitch in one paragraph

You begin as a child Sans-Nom, before the Bushitaka has been broken, before the dynasty has chosen its end. The story is generated turn by turn by a Narrator agent with deep canonical passion who refuses to compromise the IP's sacred laws — but otherwise has total freedom. Each character you meet is its own actor agent with its own evolving understanding of you, scoped only to what that character has actually witnessed. There is no win condition. There is only what you become before you die.

## Top-level subtrees

| Subtree | Owner | What lives here |
|---|---|---|
| [`principles/`](principles/README.md) | Designer | Vision pillars, player fantasy, tonal fidelity, why this team |
| [`mechanics/`](mechanics/README.md) | Designer | Core loop, verbs, input, player character, cast, memory model, world |
| [`architecture/`](architecture/README.md) | Lead engineer | Agent topology, Narrator/actor agents, Honcho integration, stack, ADRs |
| [`phases/`](phases/README.md) | Producer + Engineer | Phase 0 (preproduction) → Phase 3 (dynasty's weight) + v1.0 |
| [`project/`](project/README.md) | Producer | Team roles, risks, cost, monetization, decisions log |

## Full leaf index

### Principles

- [Pillar 1 — The IP is law](principles/pillars/01-ip-is-law.md)
- [Pillar 2 — Characters are witness-bound](principles/pillars/02-characters-witness-bound.md)
- [Pillar 3 — The Narrator is an author](principles/pillars/03-narrator-as-author.md)
- [Pillar 4 — Replay is the medium](principles/pillars/04-replay-is-medium.md)
- [Pillar 5 — Prose first](principles/pillars/05-prose-first.md)
- [Player fantasy](principles/player-fantasy.md)
- [Tonal fidelity](principles/tonal-fidelity.md) — how the original 80-person GDD pillars survive
- [Why this team](principles/why-this-team.md)

### Mechanics

- [Core loop](mechanics/core-loop.md)
- **Verbs**: [read](mechanics/verbs/read.md) · [choose](mechanics/verbs/choose.md) · [speak/act](mechanics/verbs/speak-act.md) · [ask Tonto](mechanics/verbs/ask-tonto.md) · [reflect](mechanics/verbs/reflect.md) · [recall](mechanics/verbs/recall.md) · [wait](mechanics/verbs/wait.md)
- **Input**: [free text vs. choices](mechanics/input/free-text-vs-choices.md) · [interpretation prompt](mechanics/input/interpretation-prompt.md) · [edge cases](mechanics/input/edge-cases.md)
- **Player character**: [seed](mechanics/player-character/sans-nom-card-seed.md) · [card evolution](mechanics/player-character/card-evolution.md) · [multiple Sans-Noms / save model](mechanics/player-character/multiple-sans-noms.md) · [death state](mechanics/player-character/death-state.md)
- **Cast**: [overview](mechanics/cast/README.md) · [aghora](mechanics/cast/aghora.md) · [tonto](mechanics/cast/tonto.md) · [steelhead](mechanics/cast/steelhead.md) · [vicenta](mechanics/cast/vicenta.md) · [aghnar](mechanics/cast/aghnar.md) · [oda](mechanics/cast/oda.md) · [othon](mechanics/cast/othon.md) · [honorata](mechanics/cast/honorata.md) · [lothar](mechanics/cast/lothar.md) · [generated NPCs](mechanics/cast/generated-npcs.md) · [dead and absent](mechanics/cast/dead-and-absent.md)
- **Memory model**: [Honcho mapping](mechanics/memory-model/honcho-mapping.md) · [sessions](mechanics/memory-model/sessions.md) · [card updates](mechanics/memory-model/card-updates.md) · [card eviction](mechanics/memory-model/card-eviction.md)
- **World**: [Bushitaka](mechanics/world/bushitaka.md) · [IP commitments](mechanics/world/ip-commitments.md)

### Architecture

- [Stack](architecture/stack.md) — single source of truth, resolved by ADR 0001
- [Agent topology](architecture/agent-topology.md)
- **Narrator agent**: [peer card](architecture/narrator-agent/peer-card.md) · [authoring loop](architecture/narrator-agent/authoring-loop.md) · [failure modes](architecture/narrator-agent/failure-modes.md) · [dev-loop ergonomics](architecture/narrator-agent/dev-loop-ergonomics.md)
- **Actor agents**: [invocation](architecture/actor-agents/invocation.md) · [card seeding](architecture/actor-agents/card-seeding.md)
- **Honcho integration**: [peers](architecture/honcho-integration/peers.md) · [sessions](architecture/honcho-integration/sessions.md) · [representations](architecture/honcho-integration/representations.md) · [dreaming](architecture/honcho-integration/dreaming.md)
- [Lore retrieval](architecture/lore-retrieval.md) — incl. comic corpus ingestion plan
- [Repo structure](architecture/repo-structure.md)
- [Python sidecar](architecture/python-sidecar.md)
- [Observability](architecture/observability.md)
- **ADRs**: [0001 Go vs. TypeScript](architecture/decisions/0001-go-vs-typescript.md) · [0002 Honcho hosted vs. self](architecture/decisions/0002-honcho-hosted-vs-self.md) · [0003 streaming Narrator output](architecture/decisions/0003-streaming-narrator-output.md) · [0004 card eviction policy](architecture/decisions/0004-card-eviction-policy.md)

### Phases

- **Phase 0 — Pre-production**: [scope](phases/phase-0-preproduction/scope.md) · [deliverables](phases/phase-0-preproduction/deliverables.md) · [exit criteria](phases/phase-0-preproduction/exit-criteria.md)
- **Phase 1 — Vertical slice**: [scope](phases/phase-1-vertical-slice/scope.md) · [slice spec](phases/phase-1-vertical-slice/slice-spec.md) · [beats](phases/phase-1-vertical-slice/beats.md) · [exit criteria](phases/phase-1-vertical-slice/exit-criteria.md)
- **Phase 2 — Adolescence**: [scope](phases/phase-2-adolescence/scope.md) · [beats](phases/phase-2-adolescence/beats.md) · [exit criteria](phases/phase-2-adolescence/exit-criteria.md)
- **Phase 3 — Dynasty's weight**: [scope](phases/phase-3-dynasty-weight/scope.md) · [encounters](phases/phase-3-dynasty-weight/encounters.md) · [exit criteria](phases/phase-3-dynasty-weight/exit-criteria.md)

### Project

- **Team**: [lead engineer](project/team/lead-engineer.md) · [narrative designer](project/team/narrative-designer.md) · [frontend / UX](project/team/frontend-ux.md) · [producer / playtest](project/team/producer-playtest.md)
- **Risks**: [register](project/risks/register.md) · [license tracks](project/risks/license.md)
- **Cost**: [per-turn model](project/cost/per-turn-model.md) · [per-phase burn](project/cost/per-phase-burn.md) · [monetization options](project/cost/monetization-options.md)
- [Decisions log](project/decisions-log.md) — cross-cutting open Qs

## Source assets

- `external/comics/meta-barons/` — 446 Metabarons comic page scans (raw input for the [lore corpus ingestion plan](architecture/lore-retrieval.md))
- `external/references/council of castaka/metabarons/` — Council of Castaka character cards + relations matrix + invocation prompts + Go source. Architectural prototype for sans-nom's actor-orchestration layer; canonical character cards seed the actor peers.

## How to navigate

- **New to the project**: read [`principles/`](principles/README.md) start to finish (~15 min). Then skim [`mechanics/core-loop.md`](mechanics/core-loop.md) and [`architecture/agent-topology.md`](architecture/agent-topology.md). You'll have the shape.
- **Designer working on a character**: [`mechanics/cast/<character>.md`](mechanics/cast/README.md) is your file. Cross-link to the Council of Castaka card under `external/references/`.
- **Engineer starting Phase 0**: read [`phases/phase-0-preproduction/`](phases/phase-0-preproduction/scope.md), then [`architecture/`](architecture/README.md) end to end.
- **Producer planning the next phase**: [`phases/<phase>/exit-criteria.md`](phases/README.md) tells you what gate must pass; [`project/cost/`](project/README.md) tells you the burn.

## Conventions

- Architecture decisions go in [`architecture/decisions/`](architecture/decisions/) as ADRs (numbered, append-only, status field).
- Cross-cutting open questions go in [`project/decisions-log.md`](project/decisions-log.md).
- Per-phase docs follow the pattern: `scope.md` (what's in/out) + `exit-criteria.md` (gates to next phase) + optional supporting docs (`beats.md`, `slice-spec.md`, `deliverables.md`).
- Cast files are not duplicates of the Council of Castaka cards — they are game-side overlays that link to the lore card as source of truth.
