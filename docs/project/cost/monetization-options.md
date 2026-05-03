# Monetization Options

Three viable models. The team picks one (or hybridizes) at the start of Phase 3, after Phase 1 and Phase 2 data have informed the choice.

## Option 1 — Bring-your-own-key (BYOK)

**What it is.** Players supply their own Anthropic API key. The studio's LLM cost burden is removed entirely. Player pays Anthropic directly per their actual usage.

**Pricing model.** Studio offers the game as a one-time purchase ($20-$30 likely range) with no recurring cost. The game asks for the player's key on first launch; key is stored locally / in their account.

**Pros.**

- Studio cost is bounded to infra (Honcho hosted, web hosting, support). Predictable.
- Players who play heavily pay accordingly to Anthropic; players who play lightly pay accordingly. Naturally fair.
- Standard for AI-narrative indies (NovelAI, AI Dungeon's tiers). Players in the AI-narrative segment expect this option.

**Cons.**

- Friction at signup. Some players will bounce at the "supply your own API key" step.
- Studio has no per-turn revenue.
- Requires the player to have an Anthropic account, which has its own friction.

**Best for.** Players in the AI-narrative segment who already have keys. Also the structural escape hatch if other models don't pencil.

## Option 2 — Subscription

**What it is.** $X/month gives Y turns/mo. Industry-standard for the segment.

**Pricing model.** Tier 1: ~$15/month for ~200 turns. Tier 2: ~$30/month for ~600 turns. Above-tier usage either rolls over, degrades to a cheaper model, or upcharges.

**Pros.**

- Recurring revenue.
- Studio captures margin between subscription price and actual LLM cost (if usage stays within tier).
- Standard payment UX for players.

**Cons.**

- Studio carries LLM cost up to subscription cap. Low-usage subscribers subsidize high-usage subscribers.
- Cap-management UX is tricky. Players hate hitting "you've used your tokens" walls.
- Churn tracking and dunning are operational concerns.

**Best for.** Players who play regularly and want a flat predictable cost.

## Option 3 — Single purchase + capped runs

**What it is.** $X buys you N full playthroughs of credits. Premium model.

**Pricing model.** ~$30 for ~5 full playthroughs of credits (~3,600 turns). Once consumed, player buys more credits or upgrades.

**Pros.**

- One-time payment psychology (no subscription).
- Margin is per-purchase, not per-month.
- Players who buy and don't play heavily fund the cost-of-goods for those who do.

**Cons.**

- Studio fronts the per-turn cost; profitable only if margin from non-playing buyers covers heavy users.
- Hard to tune the credit-cost ratio without real data.
- Re-purchase friction is a churn risk.

**Best for.** Players who want to "own" the game outright.

## Hybrid

Possible: BYOK as the entry-level option (free/cheap to start, pay your own LLM cost) + subscription as the premium tier (studio handles LLM cost). Players who try BYOK and like it can upgrade to subscription. This captures both segments.

## Decision criterion

End of Phase 2 / start of Phase 3:

- If beta data shows per-turn cost is materially below projection → subscription is more attractive (margin works).
- If per-turn cost matches or exceeds projection → BYOK is the safer commitment.
- If the team has bandwidth for hybrid complexity → hybrid is best long-term.

The architecture supports all three from day one — see [stack.md](../../architecture/stack.md). The decision is a business call, not an engineering one.

## Out of scope

- Microtransactions. The IP rejects this; the design rejects this.
- Ad-supported. Same.
- Free-to-play with energy timers. Same.
- Studio takes a cut of player Anthropic billing in BYOK mode. Anti-pattern; players hate this.
