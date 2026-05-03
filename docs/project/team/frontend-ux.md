# Frontend / UX Engineer

## Owns

- The web frontend (React or Svelte + Tailwind, designer's pick — see [stack.md](../../architecture/stack.md)).
- The chat-shaped UI: prose pane, choice list, free-text input, scrollback.
- The Tonto-as-codex sidebar.
- Image display affordances (Phase 2 — opt-in toggle, image rendering, caption styling).
- Audio playback affordances (Phase 3 — opt-in toggle, per-character voice, playback controls).
- Workspace lifecycle UI: new game, resume, "boys who lived" archive view.
- Death-state UX (the moment a workspace closes — designed to honor the boy, not punish the player).

## Collaborates with

- **Lead engineer** on the WebSocket / SSE protocol for streaming Narrator output. See [ADR 0003](../../architecture/decisions/0003-streaming-narrator-output.md).
- **Narrative designer** on prose-pane typography, italic interiority rendering, scene pacing.
- **Producer** on beta-cohort onboarding flow, telemetry events from the frontend.

## Phase responsibilities

- **Phase 0** — not on critical path; can scaffold Phase 1 web UI in parallel.
- **Phase 1** — web UI from MVP to beta-ready. Streaming prose. Choice + free-text input. Scrollback. Workspace lifecycle.
- **Phase 2** — image display + opt-in toggle, location-arc UI affordances, death-state UX.
- **Phase 3** — TTS opt-in + audio playback, monetization checkout flow (or BYOK key entry UI).

## Design principles

- Prose first. The UI's primary affordance is the prose pane.
- No game-y chrome. No quest log. No HUD. No XP bar. Italic interiority is the only typographic flourish.
- Sparse. Affordances appear when needed; disappear otherwise.
- Honest. If image gen costs money, the toggle says so. If TTS adds latency, the toggle says so.
- Accessible by default. Reading is the medium; the prose must be readable for everyone.

## Hiring profile

- Has shipped at least one production web app. Familiar with React or Svelte at depth (preferably both, to argue the choice).
- Cares about typography. Can talk about line-height, measure, and reading rhythm without prompting.
- Comfortable with WebSocket / SSE and streaming UIs.
- Sensitivity to tone. Can argue that a death state should be quiet, not loud, without needing a brief.

## Anti-pattern

An engineer who builds chat-app UX with avatars and message bubbles. This is a *book-with-input*, not a chat. See [Pillar 5](../../principles/pillars/05-prose-first.md).
