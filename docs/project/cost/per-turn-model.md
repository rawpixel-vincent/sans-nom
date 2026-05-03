# Per-Turn Cost Model

## The math

Per turn, with ~3 actors involved on average:

- 1 Narrator call (Opus tier): **~$0.05**
- 3 actor calls (Sonnet tier): **~$0.015**
- 1 Honcho writeback: negligible per call
- Honcho reasoning pipeline: **~$0.005 per message**

**Total per turn: ~$0.07.**

## Per playthrough

A 4-hour playthrough at ~3 turns/minute is ~720 turns.

**~$50 per full playthrough.**

A shorter playthrough (~2 hours, ~360 turns) is ~$25. A longer one (~10 hours, ~1800 turns) is ~$125.

## Variance drivers

- **Number of actors per turn.** Cap is 4 (see [failure-modes.md](../../architecture/narrator-agent/failure-modes.md)). Scenes with 4 actors cost ~$0.085; scenes with 1 actor cost ~$0.06.
- **Lore retrieval depth.** Per-turn search is k=5 by default. Bumping to k=10 increases context cost but also risks crowding the Narrator's context.
- **Narrator scene length.** Long scenes (Narrator running 4-paragraph prose) cost more output tokens. The peer card's *"keep scenes short by default"* instruction is a cost lever as well as a tone lever.
- **Tonto storytelling.** A long Tonto story (10 minutes of reading) is one big actor call — easily $0.05 alone.
- **Image generation (Phase 2+).** Adds variable cost per opt-in image. Flux Pro / Imagen pricing varies; estimate $0.02-$0.05 per image.
- **TTS (Phase 3+).** ElevenLabs per-character-voice cost. Estimate $0.10-$0.30 per minute of generated audio.

## Cost by tier

If model selection shifted (e.g., Narrator dropped from Opus to Sonnet for cost):

- Narrator on Sonnet: ~$0.015 per call → **per-turn cost ~$0.035**
- Per-playthrough cost ~$25 instead of $50

This is the major lever if cost becomes a problem. The trade is voice quality. Phase-1 evaluation should include a Sonnet-Narrator A/B test against the regression corpus to know whether the trade is acceptable.

## What this is for

- Phase-by-phase budget projection (see [per-phase burn](per-phase-burn.md)).
- Monetization pricing (see [monetization options](monetization-options.md)).
- Operational cost-discipline alerts (per-turn cost moving above threshold triggers investigation).

## What this is *not*

- Not a precise prediction. Real per-turn cost will vary by ±30% on individual playthroughs based on the variance drivers above.
- Not a hard cap. The system does not refuse to run a turn that would exceed a cost projection. (Per-tester turn caps in beta are a separate, deliberate limit.)
- Not a static target. As Anthropic adjusts pricing, this document needs revision. Maintain quarterly.
