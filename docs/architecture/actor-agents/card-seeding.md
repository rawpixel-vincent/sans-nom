# Actor Card Seeding

Where actor peer cards come from at workspace creation, and how the seed/evolution boundary is enforced.

## Seed sources

- **Canonical characters** (Aghora, Steelhead, Vicenta, Aghnar, Oda, Othon, Honorata, plus Sans-Nom-as-adult for canon reference) — seed verbatim from the Council of Castaka library at `external/references/council of castaka/metabarons/`. Each card is a Phase-0 deliverable: copy + lightly edit if needed (the Council cards were written for adult characters in dramatic deliberation; the game may need to seed *young* versions for some characters).
- **Tonto** — no existing card. Phase-0 deliverable: write from scratch, drawing on comic narration sequences and Council `council_summoning_prompts.md` Tonto entries. See [tonto cast file](../../mechanics/cast/tonto.md).
- **Lothar** — no existing card. Phase-0 deliverable: same as Tonto but smaller scope. See [lothar cast file](../../mechanics/cast/lothar.md).
- **Generated NPCs** — Narrator generates the card at first appearance. See [generated-npcs](../../mechanics/cast/generated-npcs.md).

## Workspace bootstrap

The `seed-honcho` script (one-shot, run at workspace creation) loads each card file, creates the corresponding peer in Honcho, and sets the peer card via `set_card`. For directional cards (the relationship matrix), it loads from `lore/relations/` and calls `set_card(target=...)` for each pair.

The Council of Castaka relationship matrix (full 56-pair directional matrix at `external/references/council of castaka/metabarons/relations/`) is the seed for the directional representations. This is the single largest leverage point from existing assets — the dynasty's refraction matrix is already written.

## Seed/evolution boundary

The actor card seed is **immutable per workspace**. It loads at workspace creation and is never edited during play. Evolution happens through Honcho's pipeline (dreaming + Narrator-triggered updates), which appends or replaces cards via the Honcho SDK — never by re-reading the seed file mid-playthrough.

Why: this is the [actor-bleed defense](../narrator-agent/failure-modes.md). If the seed could be edited mid-play, contamination from misinterpreted scene-setup prompts could propagate into the seed.

## Cross-workspace seed updates

If the team revises a canonical character card during Phase 0 or 1, *new* workspaces pick up the new seed. *Existing* workspaces continue with their old seed plus accumulated evolution. This means seed revisions do not break in-progress playthroughs.

If a revision is critical (canon error, IP commitment violated by the seed), the team can run a `re-seed` admin tool against existing workspaces — but this is a hard reset of that actor's accumulated card and should be rare. The tool emits a telemetry event and is logged.

## What gets versioned

- Card files in `lore/` are git-versioned. Every seed change is a commit.
- Per-workspace evolved card state is in Honcho. Honcho's own audit log captures evolution history.

## Phase 0 card-seeding deliverables

See [Phase 0 deliverables](../../phases/phase-0-preproduction/deliverables.md). Summary:

- All 8 canonical character cards copied + adapted to game-side (young versions where applicable).
- Tonto card written from scratch.
- Lothar card written from scratch.
- Relationship matrix loaded into directional cards.
- `seed-honcho` script runs end-to-end against a clean Honcho workspace.
