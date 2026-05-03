# Pillar 5 — Ship the prose first; image and audio are polish layers

Phase 1 is text-only and beautiful at it. Phase 2 adds opt-in scene illustrations. Phase 3 adds optional audio narration. Each phase ships standalone.

## What this means in practice

- The Narrator's prose carries the entire experience in Phase 1. UI is a chat-shaped surface with a scrollback, a choice list, and a free-text input. That's it.
- Image generation (Phase 2) is *opt-in*, *one image per scene maximum*, and *style-locked* via prompt prefix. It supplements; it does not replace the prose.
- Audio narration (Phase 3) is *opt-in* and *per-character voiced*. Same logic: supplement, not replace.
- Each phase is shippable. If Phase 2 slips, Phase 1 is still a complete game. If Phase 3 never ships, Phases 1+2 are still a complete game.

## What this rules out

- Cinematic cutscenes. The Narrator does not do staging direction.
- Voiced-first writing. Lines are not written for actors; they are written for the page.
- Image-driven scene composition. The text decides what the scene is; the image, when present, is one frame *of* that scene.

## What depends on this

- The [phase ordering](../../phases/README.md) — image and audio cannot pull forward into Phase 1 even if budget allows. The voice has to lock first.
- The [stack](../../architecture/stack.md) — primary backend is text-pipeline; image and audio are routed through the [Python sidecar](../../architecture/python-sidecar.md) so they can be added without disturbing the agent loop.
- The frontend's progressive enhancement model — image and audio toggle on top of the text view, never inline-required.
