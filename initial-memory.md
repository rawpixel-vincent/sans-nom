Purpose & context
Vinni is a developer building an agentic swarm orchestrator with a terminal interface, organized into three layers: a conversational Layer 1 (Tonto), a plan decomposition Layer 2, and a task execution Layer 3. The app integrates OpenAI and Anthropic providers directly, with OpenRouter accessible via the OpenAI provider interface.
A major completed project is the "Council of Castaka" — a slash-command gadget that summons characters from the Metabarons comic saga (Jodorowsky & Giménez) to deliberate on decisions, vote, or solve problems. This has been open-sourced at github.com/delta-9/council-of-castaka.
Vinni has deep familiarity with the Metabarons lore and characters (Othon, Honorata, Aghnar, Oda, Steelhead, Doña Vicenta, Aghora, Sans-Nom), treating them as serious creative and technical material rather than novelty.

Current state
The Council of Castaka feature is complete and shipped. Key implemented elements include:

Per-character system prompt cards with psychological depth and LLM-formatted character profiles
A full 56-pair directional relationship matrix with refracted knowledge
Per-member summoning prompts targeting each character's specific psychological weakness to force concrete answers
Council invocation prompt with tension mechanics
Three council modes: one-shot, peer-pressure, and scrutiny (where members receive all others' final statements before committing)
Model assignments: Claude Sonnet for most council members, Claude Opus for Sans-Nom, MiniMax for Tonto (conversational role)
Two distinct Tonto roles: planner (Layer 1 orchestrator, translates user intent into technically grounded briefs for Layer 2) and conversational RP (Claude Opus, full lore in context, ask_metabaron tool, council orchestration, fourth-wall [Tonto, aside] asides, post-council analysis tied to each baron's specific wound)
A pixel art Castaka crest loader animation (24×24, four-color palette, phase-sequential reveal, looping with fade)
A visual reference guide aggregating portrait image URLs across multiple sources (Bedetheque, Humanoids, Writeups.org, ComicArtFans, Heritage Auctions, Corner4art, DeviantArt)


Key learnings & principles

Layer separation is important and precise: Layer 1 translates intent into technically grounded briefs; it does not decompose into work units (that's Layer 2). Vinni corrects scope misattributions clearly when they occur.
Character psychological weaknesses are the lever for extracting concrete, in-character answers — not general prompting.
Scrutiny mode (each member sees all others' final statements before committing) produced notably stronger outputs than simpler modes.
The council proved genuinely useful tested against both product decisions and open-ended philosophical questions.


Approach & patterns

Works iteratively and directly; expects Claude to build on prior context without re-explaining established ground.
Corrects scope and responsibility boundaries clearly when Claude drifts (e.g., misattributing functions between layers).
Prefers going deep into lore, mechanics, and character psychology rather than surface-level treatments.
Combines creative/narrative work (character design, lore) tightly with technical implementation (prompt engineering, tool architecture, UI animation).
Before implementation, scan for existing functions/helpers that can be reused instead of duplicating behavior across modules. At the end of each phase, include a cleanup pass that removes dead code and throwaway tests left behind by the work.
Canon extraction artifacts are protected source data at every development stage: preserve source refs, review trails, and structured intermediate layers because they enable later fact-card extraction and experimentation.
After canon extraction, the next major focus is the writer/narrator loop: model choice, context assembly, lore retrieval tools, character-store selection, and an initial story-seeding turn. A useful split is a lore-heavy "nerd" verifier that protects Metabarons invariants and a less-encoded scenarist that creates new main characters/core lines without merely replaying canon.


Tools & resources

Providers: OpenAI, Anthropic, OpenRouter (via OpenAI interface), MiniMax
Models in active use: Claude Opus (Sans-Nom, Tonto RP), Claude Sonnet (most council members), MiniMax (Tonto conversational). For future Claude model aliases, use sonnet[1m] and opus[1m] when the task asks for Sonnet or Opus.
Open-source repo: github.com/delta-9/council-of-castaka
Reference sources: Bedetheque, Humanoids, Writeups.org, ComicArtFans, Heritage Auctions, Corner4art, DeviantArt
