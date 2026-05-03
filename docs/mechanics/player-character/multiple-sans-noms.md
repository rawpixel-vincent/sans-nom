# Multiple Sans-Noms — Save Model

A single Honcho workspace contains one Sans-Nom (one playthrough). Different playthroughs are different workspaces. Each Sans-Nom is a separate boy.

## Why workspace-per-playthrough

Players who replay are not playing the same boy with different choices — they are playing a different boy. This is structurally important. In a saga about the failure of cycles to repeat exactly, replay-as-fresh-character is the canonical-feeling mechanic. A New Game+ that carried over abilities would be wrong for the IP. See [Pillar 4](../../principles/pillars/04-replay-is-medium.md).

Workspace-per-playthrough also means the directional representations are per-boy. Aghora's representation of *this* Sans-Nom is hers and his alone. There is no global Aghora-of-Sans-Nom that drifts across runs.

## Save / resume / cross-device flow

This was an open question in the original draft (Appendix B.3). Concretized:

- **New game** = create a new Honcho workspace with a generated ID. Seed actor cards from the workspace template (the eight canonical cards + Tonto + Lothar). Seed Sans-Nom from [the seed](sans-nom-card-seed.md).
- **Resume** = workspace selector on entry. The player picks from their list of in-progress boys (each labeled with age, location-arc, last-played date).
- **Cross-device** = Honcho's hosted backend handles this naturally via account-bound workspaces. The player signs in; their workspaces follow. To be verified end-to-end in [Phase 1](../../phases/phase-1-vertical-slice/exit-criteria.md).
- **Death** = the workspace is *not* deleted. It is archived in a "boys who lived" view. Players can re-read their dead boys' stories via [Recall](../verbs/recall.md). They cannot continue them.

## What this rules out

- Save slots within a single workspace. There is one boy per workspace, not three.
- Branch points / "load before this choice" affordances. The river only flows downstream. If you want a different outcome, you grow a different boy.
- Cross-workspace memory leaks. No actor agent ever sees the contents of a workspace it doesn't belong to.

## What this enables

- Family tree views (long-term): a player might have raised five Sans-Noms; the UI can present them as five boys whose stories diverged. Out-of-scope for v1, structurally available for v2.
- Multiplayer sans-noms (long-term): two players play different boys in different timelines, with their stories optionally aware of each other. Architecturally available; out of scope for v1.
