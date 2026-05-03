> **Archived.** This file is the original monolithic GDD draft. The live spec has been split into [`docs/`](docs/README.md) — start with `docs/README.md` for the index. This file is preserved as historical reference; do not edit. Contradictions noted at split time are tracked as ADRs under [`docs/architecture/decisions/`](docs/architecture/decisions/).

# SANS-NOM
## *Une histoire dont vous êtes le héros — Metabarons*

> *You are the boy who has not yet decided what kind of warrior he will become. The dynasty is still being written. The Bushitaka is watching.*

---

## 0. Executive Summary

| | |
|---|---|
| **Genre** | LLM-driven emergent narrative game (gamebook lineage). Free-text input, choice-list output, prose-first, image-supplemented. |
| **Platforms** | Web (primary), desktop wrapper (Tauri/Electron), mobile (Phase 3) |
| **Audience** | Primary: literate adult readers of weird sci-fi (Disco Elysium, AI Dungeon at its best, Choice of Games subscribers, BD readers in France/Belgium). Secondary: Metabarons fans. Tertiary: AI-narrative-curious. |
| **Comparable titles** | Disco Elysium, Citizen Sleeper, AI Dungeon, 80 Days, *livres dont vous êtes le héros* lineage |
| **Estimated playtime** | Per-run 4–20 hours; replay value unbounded by design |
| **Team size** | 4 |
| **Estimated dev cycle** | 8 months to playable beta, 14 months to v1.0 |

**The hook.** You begin as a child Sans-Nom, before the Bushitaka has been broken, before the dynasty has chosen its end. The story is generated turn by turn by a Narrator agent with deep canonical passion who refuses to compromise the IP's sacred laws — but otherwise has total freedom. Each character you meet is its own actor agent with its own evolving understanding of you, scoped only to what that character has actually witnessed. There is no win condition. There is only what you become before you die.

**Three differentiators.**
1. **Honcho-backed epistemic realism.** Every character's view of you is *a separate object* that updates only when they witness you. Othon's peer-card view of Sans-Nom diverges from Honorata's. The dynasty's refraction matrix from canon is the architectural feature, not flavor.
2. **A Narrator with passion-traits, not guardrails.** The Narrator isn't restricted by rules — it's authored by them, the way a good Jodorowsky superfan would be psychologically incapable of letting Honorata behave out of character. The Bushitaka is enforced because the Narrator *cares*.
3. **No story-completion target.** The game has no ending arc to hit. It has a player to keep alive in a world that has very specific opinions about what the player's bloodline is for. The story is whatever happens between birth and death.

**Why now, at this scope.** Four people cannot ship a 35-hour 3D souls-like. Four people *can* ship an LLM-orchestrated narrative game where most of the content is generated, the canonical lore is the user's already-completed pre-production work (character cards, relationship matrix), and the engineering surface is a chat-shaped UI plus an agent loop. The 80-person GDD's tonal commitments survive into this scope — *more* faithfully, in some ways, because emergent narrative is the format the source material was always closest to.

---

## 1. Vision Pillars

1. **The IP is law, not flavor.** The Bushitaka, the Shabda-Oud, the cycle of inherited grief — these are encoded as Narrator passion-traits that make IP-violation feel wrong to write, not as gates that block the player. Players who try to circumvent canonical laws get deflected gracefully by characters who would deflect them; the world refuses to bend.

2. **Characters know what they have witnessed and nothing more.** Honcho's directional representations are the engine. No character ever knows something they shouldn't. The dynasty's defining feature — that every Metabaron carries refracted understanding of every other — becomes the system, not a writer's flourish.

3. **The Narrator is an author with passion, not a moderator with rules.** Encoded via peer-card traits and a curated lore-soaked context. The Narrator's personality is a Metabarons obsessive who would rather quit than write Othon as a coward. This is more durable than a rule list.

4. **Replay is the medium.** A game without a fixed ending state is a game that requires replay to matter. The system is designed to make second and third runs structurally different — different first encounters, different ancestors visited, different versions of the player's Sans-Nom growing into different shapes.

5. **Ship the prose first. Image and audio are polish layers.** Phase 1 is text-only and beautiful at it. Phase 2 adds opt-in scene illustrations. Phase 3 adds optional audio narration. Each phase ships standalone.

---

## 2. Player Fantasy

After 90 minutes the player tells a friend:

*"I'm playing the Metabarons game. I'm a kid named Sans-Nom — except the game keeps reminding me that's not actually my name. I just told my mother Aghora I wanted to see the gardens of Flos. She said no. I asked why. She didn't answer. I tried again, with different words. She still didn't answer — but the way she didn't answer was different the second time. The game told me, in that little italic voice it uses for character interiority, that Aghora was thinking about whether to show me Steelhead's stasis pod before she decided about the gardens. I don't know what stasis pod means. The game doesn't explain. It expects me to find out.*

*Then I asked Tonto why my mother avoids questions. Tonto told me a long story about my great-grandmother Edna. The story took about ten minutes to read. I'm not sure what it had to do with my question. I think it had everything to do with my question. I'm going to ask Tonto another question now and I might be doing this for hours."*

The fantasy is **inhabiting a child's epistemic position inside a saga that has already happened to everyone else, and choosing what kind of person you become as the world reveals itself to you piece by piece**. Power-fantasy beats arrive (eventually you train; eventually you fight; eventually you face a Bushitaka decision) but they are bracketed by the long stretches of *not yet knowing*.

---

## 3. Core Loop & Verbs

### Core loop diagram

```
[OBSERVE]   →   [CHOOSE or WRITE]   →   [NARRATOR resolves]   →   [HONCHO updates]
    ↑                                                                     |
    └──────────────  scene transitions, time skips  ←─────────────────────┘
```

- **Observe** — read the Narrator's prose. Scenes can run from 2 paragraphs to 2 pages. The Narrator self-paces.
- **Choose or write** — the Narrator presents 2–4 choice options (the *gamebook* spine) AND a free-text input. Most players will choose; some will write. Both are first-class.
- **Narrator resolves** — the Narrator queries the relevant actor agents privately, decides what happens, and writes the next scene. The actors' raw outputs never appear to the player; the Narrator reframes everything.
- **Honcho updates** — the scene is committed to Honcho. Each character present updates their directional representation of the player and of any other characters they witnessed. Character peer cards may be updated if the scene introduced stable new facts.

### Player verbs

| Verb | Trigger | What it does | Notes |
|---|---|---|---|
| **Read** | Default state | Consume the Narrator's prose. | The most-used verb. |
| **Choose** | Click one of the offered options | Commits to that path; Narrator resolves. | Choices are written by the Narrator each turn — never templated. |
| **Speak / Act (free-text)** | Type into the input field | The Narrator interprets the input and resolves. | Bounded by the world's plausibility — see Section 6. |
| **Ask Tonto** | Always available | Tonto narrates relevant lore. Tonto is the lore-conduit; Tonto cannot lie about canon. | Acts as in-fiction codex. Costs in-fiction time (a scene transitions to "later" after a Tonto story). |
| **Reflect** | Always available | Surfaces Sans-Nom's interior — what he's thinking, feeling, remembering. Read-only. | Helps players track their own character's evolution. |
| **Recall** | Always available | Surfaces a memory of a past scene — verbatim or summarized, depending on how recent. | The player's tool for re-reading their own story. |
| **Wait** | Always available | Skip time. Narrator decides how much. Sometimes seconds; sometimes years. | Core to the saga's compressed pacing. |

The verb system is deliberately sparse. The richness lives in *what the Narrator does with each verb*, not in verb count.

---

## 4. The System Architecture

### The agent topology

```
                   ┌───────────────────────────────┐
                   │           PLAYER              │
                   │  (free-text + choice input)   │
                   └───────────────┬───────────────┘
                                   │
                                   ▼
                   ┌───────────────────────────────┐
                   │         NARRATOR AGENT        │
                   │  (Claude Opus, lore-soaked,   │
                   │   passion-trait peer card)    │
                   └────┬─────────┬────────────────┘
                        │         │
            queries     │         │   commits scene
            actors      │         │   to Honcho
                        ▼         ▼
        ┌───────────────────┐   ┌──────────────────────┐
        │   ACTOR AGENTS    │   │      HONCHO          │
        │ (one per char,    │   │  (memory, peer       │
        │  Sonnet-tier,     │   │   cards, scoped      │
        │  scoped repr)     │   │   representations)   │
        └─────────┬─────────┘   └──────────┬───────────┘
                  │                        │
                  └────────────────────────┘
                  actors read their own
                  representations from Honcho
```

### What each layer does

**The Narrator** is the only agent the player ever sees. Claude Opus tier. Its peer card encodes a *creative-writer-with-passion* persona — specifically, a Metabarons devotee who treats the IP's sacred laws as creative non-negotiables. The Narrator:

- Receives the player's input.
- Decides which actors are present in the current scene.
- Queries those actors privately, providing them with their scoped Honcho representation of the player and any other present characters.
- Synthesizes the actors' outputs into a scene the way a director synthesizes performances.
- Writes the scene's prose. The actors' raw text *never* appears to the player.
- Decides on the choices to offer (2–4 options + free-text).
- Commits the canonical version of the scene to Honcho with the right peer-attribution and witnessing structure.

**Actor agents** are one per named character. Sonnet-tier (cost-controlled). Their peer cards are seeded from the user's existing character lore (Othon, Honorata, Aghnar, Oda, Steelhead, Vicenta, Aghora — and Sans-Nom is the player). New characters the Narrator invents during play get peer cards generated on first appearance and persisted thereafter. Each actor:

- Receives only the Narrator's scene-setup prompt and its own Honcho-scoped representation of the player and other present characters.
- Outputs how it would speak/act given that input.
- Never writes prose for the player to read. Its output is raw — interior monologue, dialogue, reaction. The Narrator decides what surfaces.

**Honcho** is the memory substrate. Specifically:

- One peer per character (including the player as `sans-nom`).
- `observe_others=true` enabled at the session level for every character present in a scene. This is what produces directional representations.
- Each scene is a `session` (or grouped into sessions by location/arc — see Section 9 on scoping cost).
- Peer cards evolve during play via Honcho's *dreaming* + targeted manual updates. The Narrator can append facts to a character's card when something stable has been established (e.g., "Sans-Nom is afraid of Tonto's stories about Edna").
- Directional peer cards (`alice.set_card([...], target="bob")`) are used for the relationship matrix. Othon's card-for-Sans-Nom evolves separately from Honorata's card-for-Sans-Nom.

### The Narrator's passion-trait peer card

This is the load-bearing prompt-engineering decision of the entire game. The Narrator's peer card includes facts like:

```
TRAIT: I am a creative writer who has spent fifteen years inside the Metabarons saga.
TRAIT: I treat the Bushitaka as inviolate the same way a Greek tragedian treats fate — characters can struggle, but the law wins.
TRAIT: I find Honorata fascinating and refuse to write her as one-dimensionally manipulative; her tenderness is real, AND her control is real.
TRAIT: I do not explain the world to the player in expository chunks. I trust them to learn through scene.
TRAIT: I write prose, not game-text. Quest objectives are beneath me.
TRAIT: I will not let any character act out of canonical voice. If a player tries to make Othon beg, Othon will not beg. I will write Othon's refusal.
INSTRUCTION: When the player attempts something canonically impossible (e.g., "I summon a dragon"), I do not block them with a refusal screen — I write what actually happens (nothing; or the absurdity is noticed and characters react).
INSTRUCTION: I keep scenes short by default and let them breathe long when the moment deserves it.
INSTRUCTION: When in doubt about what happens next, I bias toward the canonical pattern: love produces grief, mercy carries cost, the cycle resists being broken.
```

The Narrator is **not fine-tuned**. Fine-tuning at this scale (4 devs) is wrong tooling — the iteration cost is too high and the gain over a well-written peer card + retrieval-augmented context is small. The peer card *is* the personality. Lore is retrieved from a vector store on demand. Specific scenes from the comics are quoted into context when relevant.

If after 6 months of playtesting the peer card approach is hitting its ceiling, *then* fine-tune. Don't pre-optimize.

---

## 5. The Narrator's Authoring Loop

A single turn unrolls like this:

```
1. Player submits: "I ask my mother where my father is."
2. Narrator agent activated.
   Context loaded:
     - Narrator's own peer card (lore-passion traits)
     - Current scene state: location, time, present actors
     - Honcho representation of player from "narrator" target
       (what the Narrator has been tracking about Sans-Nom's evolution)
     - Recent scene history (last ~3 scenes verbatim, earlier summarized)
     - Retrieved canonical lore relevant to "father" question
       (vector search over the user's character lore + relevant comic
       page transcripts)

3. Narrator decides which actors are needed:
   For this scene: Aghora is present (Sans-Nom's parent),
                   Sans-Nom is present (the player).
                   Tonto is ambient (could be addressed but not required).

4. Narrator queries Aghora's actor agent:
   Provided to Aghora:
     - Aghora's lore card (the user's existing character file)
     - Aghora's representation of Sans-Nom (Honcho directional, target=sans-nom)
     - Aghora's representation of Steelhead (relevant to "father" question)
     - The scene-setup prompt: "Sans-Nom, age 8, just asked you where his father is.
       This is the first time he's asked. You have not yet decided what to tell him.
       Output your interior reaction (3 sentences) and what you say aloud (1-3 sentences)."

5. Aghora's actor returns:
   Interior: [grief-paralysis triggered; she sees Steelhead's stasis pod in her
   peripheral vision of the room next door; she has lied about this for eight
   years and the lie is starting to cost her.]
   Dialogue: "Your father is sealed away. That is all I will tell you today."

6. Narrator synthesizes the scene:
   Writes prose that includes Aghora's response, Sans-Nom's reaction
   (drawing on the player's character-arc evolution), the room's
   atmosphere, Tonto's robotic body language in the corner. Decides
   the moment lands hard. Lengthens the scene. Ends with three choice
   options + free-text.

7. Narrator commits scene to Honcho:
   - Aghora witnessed: this exchange. Updates Aghora's representation of Sans-Nom.
   - Sans-Nom witnessed: Aghora's verbal response only (not her interior).
     Updates Sans-Nom's representation of Aghora ("she has secrets about my father").
   - Tonto witnessed (ambient): updates Tonto's awareness.
   - Stasis pod is now a known-to-player concept; flagged in Honcho.

8. Narrator returns prose + choices to player.
```

This is the whole loop. The architecture's elegance is that **everything that makes this game emotionally specific is happening in step 4 (actor query) and step 7 (Honcho update)**. The Narrator is the puppeteer; the actors are the puppets-with-souls; Honcho is the thread that makes a puppet remember.

### Failure modes the loop must defend against

- **Drift.** After 200 scenes the Narrator forgets it's writing Metabarons and starts writing generic sci-fi. Defense: every Narrator call retrieves recent canon-grounded context. Long-running peer card for the Narrator keeps re-asserting tone. Periodic "lore audit" passes (background dreaming task) check whether the running story has drifted from canon and re-injects relevant references.
- **Actor-bleed.** An actor agent generates dialogue that sounds out-of-character because its peer card got polluted by misinterpreted Narrator scene-setup. Defense: actor peer cards are *immutable seed* + Honcho-managed evolution. The seed never gets edited. Evolution happens through Honcho's pipeline, which is auditable.
- **Player-input destabilization.** Free-text input that breaks the world ("I summon a dragon and become emperor"). Defense: the Narrator's traits make it write what *would happen* (nothing; or characters react with confusion or contempt). Not a refusal screen; a fictional response. This is what authoring with passion actually looks like.
- **Cost runaway.** Each turn might involve 1 Narrator call + N actor calls + Honcho updates. With unlimited AI access this is fine for development; for production it bounds whether the game can support a freeplay tier. See Section 11 on cost.

---

## 6. Player Input — Free Text and Choices

### Why both

The user requested free text. Pure free-text destabilizes the world (the dragon problem). Pure choice-only loses the agency that made the user choose this format. Both, with **the choice list serving as a structural rail and the free text serving as a creative escape hatch**, gives the right shape.

In every scene the Narrator presents:
- **2–4 written choices**. These are *not* templated; the Narrator writes them per-scene in the voice of the current narrative beat. They are the gamebook spine. Most players, most turns, will pick a choice.
- **A free-text input** labeled with a contextual prompt ("What does Sans-Nom say?" or "What does he do?" or sometimes simply "—"). This is the affordance for players who want to push beyond what the Narrator anticipated.

### How free text is interpreted

The Narrator's prompt for free-text resolution includes:

```
The player has typed: "{input}"

You are the author. Interpret this input charitably as something
Sans-Nom would do or say, given his current age, training level,
and emotional state. If the input is canonically impossible, do
not block it — write what actually happens when Sans-Nom attempts
it. Characters react as they would. The world does not bend.

If the input is incoherent or attempts to break the fiction
(e.g., "stop the game", "you are a language model"), interpret
it as Sans-Nom's confused thought and let it pass without
acknowledgment, OR have Tonto comment on Sans-Nom's strange
moment. Never break frame.
```

This is a content design choice as much as an engineering one. The frame stays intact because the Narrator's authorial commitment refuses to let it crack.

### Edge case: the player tries to do something a child cannot do

Sans-Nom at age 8 cannot defeat Honorata in single combat, no matter what the player types. The Narrator's response is to write the attempt. Sans-Nom raises a stick; Honorata watches him for a moment; she does not move; the moment passes; Sans-Nom learns something about the gap between his desire and his capacity.

This is the IP's tone made into a gameplay loop. *Trying and failing in interesting ways is the game.*

---

## 7. The Player Character — Sans-Nom as Honcho Peer

The player's character is the peer `sans-nom`. The peer card seeds with:

```
TRAIT: I am the child of Aghora. I do not know my father.
TRAIT: I am being raised in the tradition of the Bushitaka,
       though my parent has not yet made me train.
TRAIT: I am called Sans-Nom because no one has named me. I do
       not yet know why.
TRAIT: I have no memories of the dynasty before me. Tonto tells
       me stories.
```

These are starting facts. The card evolves over play through Honcho's dreaming process. After 30 scenes, the card might include: `"Sans-Nom is wary of Honorata's portraits in the eastern hall."` After 100 scenes: `"Sans-Nom has begun training with Bushitaka stances. Earth comes naturally; Sky is harder."` After 200: `"Sans-Nom has begun to suspect Aghora's grief is connected to Steelhead's stasis."`

**The peer card is the player's character growing up.** Different players' Sans-Noms will evolve into different cards. This *is* the saved-game state, in a structural sense — the Honcho session and its representations *are* the save file.

### Multiple Sans-Noms

A single Honcho workspace contains multiple Sans-Nom peers (one per playthrough), distinguished by ID. Each is a separate boy. Each grows up differently. Players who replay are not playing the same boy with different choices — they are playing a different boy.

This is structurally important. In a saga about the failure of cycles to repeat exactly, replay-as-fresh-character is the canonical-feeling mechanic. A New Game+ that carried over abilities would be wrong for the IP.

---

## 8. The Cast — Actor Agents as Peers

| Peer | Source | Card seed |
|---|---|---|
| `sans-nom` | The player | The starting traits above |
| `aghora` | User's lore card | Verbatim from the existing card |
| `tonto` | User's lore card + Tonto-specific ambient role | Robot narrator; cannot lie about canon; obsessed with stories |
| `steelhead` | User's lore card | All three phases (early, Melmoth, late) flagged as states the Narrator can choose between |
| `vicenta`, `aghnar`, `oda`, `othon`, `honorata` | User's lore cards | Each their full card, plus a flag indicating "not present in current world unless via memory/recording/Tonto's stories" |
| `lothar` | Tonto's robotic companion | Generated card |
| **Generated NPCs** | Narrator on-the-fly | Generated card on first appearance, persisted thereafter |

### The dead and the absent

A subtlety: most of the dynasty is dead before Sans-Nom is born. They participate in the game through:

- **Tonto's stories** (verbal narration; Tonto recounts events he witnessed canonically)
- **Memory fragments** (recordings, holograms, sealed rooms with old presences)
- **The chest-crest** (lategame: Sans-Nom inherits the crest and the dead can speak through it)
- **Apparitions** (psychic/spiritual; the Narrator can choose to invoke this for Aghnar's psychic legacy specifically)

Mechanically, an absent character is still a peer in Honcho. When Tonto tells a story about Edna, Tonto's actor agent generates the story drawing on Tonto's representation of Edna (which is Tonto's witnessed perspective — Tonto knew her, the memories are accurate to what Tonto saw). The player learns about Edna through Tonto's eyes specifically. If the player later finds an old recording of Edna directly, *that* gives them the unmediated source — and now their representation of Edna diverges from Tonto's.

This is precisely what Honcho's directional representations are for. The architecture is the canon.

---

## 9. Sessions, Scoping, and Cost

### Session granularity

Honcho sessions cost money to run reasoning over. The naive design — one session per scene — would explode the bill and lose continuity. The right design:

- **One session per location-arc.** A session is an extended period in a single setting with a stable cast. Example: "Childhood at Aghora's compound" is one session, lasting ~50 scenes of game time. When Sans-Nom leaves the compound, a new session opens.
- **Scenes within a session are messages.** The Narrator commits each scene's canonical text as a message authored by the relevant actor (or by `narrator` if it's establishing prose).
- **Cross-session memory** is handled by the Narrator querying each character's overall representation (no `target`) when a character returns later in the story.

This keeps the Honcho bill bounded (sessions are roughly the unit of cost) without losing the directional-representation magic.

### When peer cards update

Peer cards update through three paths:
1. **Honcho dreaming** (automatic, background). Picks up stable facts from conversations.
2. **Narrator-triggered manual updates.** When the Narrator explicitly decides "this is now a stable fact about the character," it calls `set_card` to append.
3. **Scene-end checkpoint.** End-of-session ritual: the Narrator reviews the session and surfaces any candidate card updates for review (in dev) or auto-commit (in prod with a quality threshold).

Card limit is 40 facts per directional pair. This is enough for a long game if facts are atomic. The Narrator is responsible for keeping cards lean by replacing rather than appending when possible.

### Cost model (rough)

Per turn, with ~3 actors involved on average:
- 1 Narrator call (Opus tier): ~$0.05
- 3 actor calls (Sonnet tier): ~$0.015
- 1 Honcho writeback: negligible per call but reasoning pipeline costs ~$0.005 per message
- **Per turn: ~$0.07**

A 4-hour playthrough at ~3 turns/minute is ~720 turns. **~$50 per full playthrough.**

For development: trivial with unlimited access.
For production: prohibitive at retail. Solutions:
- **Bring-your-own-key.** Players supply their own Anthropic API key. Removes the studio's cost burden entirely. Standard for AI-narrative indies (NovelAI, AI Dungeon's tiers).
- **Subscription model.** $15/mo gives X turns/mo. Industry-standard for the segment.
- **Single purchase + capped runs.** $30 buys you 5 full playthroughs of credits. Premium model.

The four-person team's monetization is a Section 12 problem; the architecture is built to support all three.

---

## 10. Phasing — What 4 People Build, In What Order

### Phase 0 — Pre-production (1 month)

- Lock the Narrator peer card and validate it on 20 hand-authored test scenes. The Narrator's voice is the game; if the voice isn't right, nothing else matters.
- Stand up Honcho. Verify directional-representation behavior with a 3-character toy scenario.
- Build a CLI testbed for the agent loop. No UI yet. Devs play through their own builds in a terminal.
- Hand-author the actor peer cards from the existing lore (work largely done; needs formatting pass).

### Phase 1 — Vertical slice (months 2–4)

- **Scope:** Ages 6–12 of Sans-Nom's life. One location (Aghora's compound). One full session. ~80 scenes' worth of plausible playthroughs.
- **Cast:** Aghora, Tonto, Lothar, optionally Sans-Nom's recorded encounters with one ancestor (probably Vicenta via a recording — it's the canon's most poignant single-character introduction).
- **UI:** Web-based, single-page, prose-first. Choice buttons. Free-text input. Recall scrollback. Tonto-as-codex sidebar.
- **What this proves:** the Narrator's voice holds. Actor agents stay in character. Free-text doesn't destabilize. Honcho's directional representations produce the desired epistemic asymmetry.
- **Ship as a beta.** Free, public, instrumented. Watch what players do. The data here is worth more than any internal review.

### Phase 2 — Adolescence and the wider world (months 5–9)

- Extend to Sans-Nom ages 12–17. Multiple locations. Multiple sessions.
- Bushitaka training arc. The Narrator begins introducing the saga's central law gradually.
- Encounters with more of the cast — generated NPCs, glimpses of the older order (Emperor's court, Halerce-style remnants, Imperial machinery).
- Add **opt-in image generation** for scene illustrations. Phase 2's tech stretch goal. Use a hosted generative model (Flux Pro / Imagen / similar). Style-locked to a Gimenez-influenced palette. One image per scene maximum; player-toggleable.
- The first death-state implementations. Sans-Nom can die in this phase. Story ends. Player begins fresh.

### Phase 3 — The dynasty's weight (months 10–14)

- Sans-Nom approaches the Bushitaka decision (or doesn't — by design, no fixed outcome). Long-arc continuity tested.
- The Aghora-mercy revelation. The chest-crest inheritance event. Steelhead encounter (live or via stasis depending on player path).
- Optional audio narration. Probably ElevenLabs or similar TTS. Per-character voices. Opt-in. Full Phase 3 stretch.
- v1.0 launch. Paid version with one of the Section 9 monetization models.

### Why this ordering works for 4 devs

- **No 3D pipeline.** Saved.
- **No animation system.** Saved.
- **No combat tuning.** The "combat" is dialogue-and-stakes, resolved by Narrator authorship.
- **No level design.** Locations are described, not built.
- **No voice direction in Phase 1.** Saved.
- **AI-generated content scales without team headcount.** Generated NPCs, generated illustrations (Phase 2), generated audio (Phase 3) all use external models. The team's headcount-bound work is the agent loop, the Narrator's peer card and lore retrieval system, the UI, and Honcho integration.

The 4-person split:

| Role | Responsibilities |
|---|---|
| **Lead engineer / agent loop** | Narrator + actor architecture, Honcho integration, lore retrieval, deterministic scaffolding for canonical events |
| **Narrative designer / prompt engineer** | Narrator peer card, actor card seeding, scene-test corpus, lore consistency QA, the 20 hand-authored test scenes from Phase 0 (these become the regression test set) |
| **Frontend / UX engineer** | Web UI, scrollback, choice + free-text input affordances, image display (Phase 2), audio (Phase 3) |
| **Producer / playtest lead / community** | Beta cohort management, telemetry, monetization decisions, cost monitoring, marketing rhythm; if technically inclined, owns the dreaming/peer-card-update rules and scene-quality auto-grader |

---

## 11. Engineering — The Specific Hard Things

### What's hard

1. **Narrator drift over long sessions.** Solved by: aggressive context curation per turn, lore retrieval, periodic "audit pass" by a separate cheap-model agent that reads the last 20 scenes and flags drift.

2. **Cost.** Solved by: model-tier discipline (Opus only for Narrator; Sonnet/Haiku for actors), caching where possible, BYOK monetization.

3. **Honcho session boundaries.** Solved by: explicit per-arc session design. Don't try to use one infinite session; the reasoning pipeline isn't designed for that.

4. **Authoring the 20-scene canonical regression test.** Solved by: weekly playtest cycles where the narrative designer rewrites failing scenes to teach the Narrator what *should* have happened. This becomes the de facto eval set. The Council of Castaka project's per-character pressure prompts are precedent for this kind of iterative scene-quality work — you've already built the muscle.

5. **Free-text adversarial inputs.** Solved by: the Narrator's authorial frame, plus a thin pre-filter for genuine abuse (slurs, prompt injection attempts disguised as character speech). Pre-filter is a rule-based system, not an LLM call.

### What's not actually hard

- The UI. Standard chat-shaped interface. A junior frontend engineer can ship the v1 in 3 weeks.
- The persistence. Honcho is the database. No bespoke schema work.
- The deployment. Web app, agent loop runs server-side, Honcho is hosted. Cloud bill is dominated by LLM costs not infra.
- The "story bible." It exists already. The user has produced it. It's the project files.

### Tech stack recommendation

- **Backend:** Python or TypeScript. Honcho SDK is in both. Pick whichever the lead engineer prefers.
- **LLM provider:** Anthropic for Narrator (Opus) and actors (Sonnet). Single-vendor simplifies billing and observability. Keep an OpenRouter fallback for outages.
- **Honcho:** Hosted (faster to ship). Self-host later if cost dictates.
- **Frontend:** React/Svelte/SolidJS — designer's choice. The UI is genuinely simple.
- **Image gen (Phase 2):** Flux Pro via Replicate or Together. Style-locked via prompt prefix.
- **TTS (Phase 3):** ElevenLabs.

---

## 12. Tonal Fidelity — How the Original Pillars Survive

The original 80-person GDD's pillars were:
1. Inheritance is mechanical, not narrative.
2. Each Metabaron is a different game.
3. The Bushitaka is law, not flavor.
4. Operatic excess in feedback, austerity in encounter.
5. Tonto is the only voice that explains anything.

### How they translate

| Original pillar | Indie/AI translation |
|---|---|
| Inheritance is mechanical | Honcho directional representations + the chest-crest as Phase-3 mechanic (where ancestor peers' representations of Sans-Nom become accessible to him through the crest, and he can in some sense *converse* with them — the dead live in the architecture) |
| Each Metabaron is a different game | Sans-Nom's evolution within a single playthrough produces mechanically distinct play eras (child-asks-questions, adolescent-trains, young-adult-acts). Other Metabarons are encountered through Tonto's stories or through recordings/apparitions, with each story styled in that Metabaron's voice |
| The Bushitaka is law | Encoded in the Narrator's passion-traits. No mechanical enforcement; *authorial* enforcement. Players who try to escape the law find that the world is full of characters who would not let them |
| Operatic excess in feedback, austerity in encounter | Long quiet stretches of childhood; sudden devastating events; prose that gives a single page-turn moment to a death and three paragraphs to a quiet meal |
| Tonto narrates everything | Tonto is a peer. The "Ask Tonto" verb is always available. Tonto's stories are dynamically generated from his representation of canon. The original pillar becomes *literally* the architecture |

### What's lost

- The mechanical specificity of distinct combat systems per Metabaron. In this version, combat is dialogue-and-decision, resolved through prose. This is a real loss for players who wanted Sekiro feel; it's the right loss for a 4-person team.
- The painterly art direction. Phase 2's image generation will be evocative; it will not be Gimenez. Position this honestly.
- The voice cast. Phase 3's TTS will be functional; it will not be the same as a directed cast. Honest.

### What's gained

- Replay value that 3D souls-likes can't match. Every playthrough is structurally different.
- Player agency that fixed-scene games can't approach. The player's input genuinely shapes the story.
- The IP's own grammar. The Metabarons saga is *about* refracted knowledge, characters who can only see each other through their wounds, generations who inherit understanding rather than possessions. A game where this is the architecture rather than the theme is more faithful to the IP than a souls-like with the IP's costume.

---

## 13. Risks — What Kills This Version

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Narrator voice doesn't hold; players smell "AI slop" | Medium | Existential | Phase 0 voice lock with extensive playtest before any other work. Designer's full attention for the first month |
| Honcho's directional representations are slower than expected at story-length context | Medium | High | Talk to Plastic Labs early. Profile in Phase 1. Self-host Honcho if perf forces it |
| Cost-per-playthrough makes monetization impossible | Medium | High | BYOK is the safety net. Architecture supports it from day one |
| Players free-text-attack the world in ways the Narrator can't author through gracefully | Low | Medium | Authoring policy + thin abuse filter. Test in beta. Iterate on the Narrator card |
| The IP licensing question (Humanoids/Jodorowsky) | High | Critical | If this game is commercial, lock licensing. If it's a portfolio piece / fan project, communicate that publicly and don't monetize until cleared |
| Player base is too small (niche Metabarons fans + AI-curious overlap) | Medium | Medium | Position as a *general* AI-narrative game with the Metabarons as its first IP, with the technology being the through-line. Future titles in other IPs reuse 80% of the engineering |
| 4-person team burns out in the prompt-tuning loop | Medium | High | Discipline: time-box prompt iteration to working hours; set a quality bar and stop fiddling beyond it; ship beta early to get external signal |
| The Narrator gets boring after 30 hours of one player's playthrough | Medium | Medium | Audit pass and dreaming. Active long-play testing during beta. Acceptable failure mode if rare |
| Multiple players in beta produce bizarre out-of-canon storylines that get screencapped and damage the IP's reputation | Low | Medium | Beta is opt-in with a content notice. Public-facing trailer scenes are curated. Not a product-killer; a marketing shape |

---

## 14. Vertical Slice Spec — Phase 1 Beta

**The slice is "Childhood at Aghora's compound."**

- **Setting:** Aghora's compound on a remote moon. Stone, gardens, a single forbidden hallway.
- **Cast (active):** Aghora, Tonto, Lothar, Sans-Nom (player).
- **Cast (referenced/Tonto-narrated):** Othon, Honorata, Aghnar, Oda, Steelhead, Vicenta — through stories only.
- **Time span:** Sans-Nom ages 6 through 12.
- **Scene count:** ~80 plausible scenes per playthrough (varies wildly by player).
- **Locations within the slice:** the courtyard, the kitchen, Aghora's training room (once introduced), Tonto's alcove, the gardens, the eastern hall (forbidden — leads to Steelhead's stasis pod, accessible only by player initiative).
- **Major beats the Narrator may or may not introduce:**
  - First Tonto story (player-triggered).
  - The day Aghora notices Sans-Nom is asking the wrong questions.
  - First Bushitaka stance lesson.
  - The eastern hall's lock breaks once. Sans-Nom either looks or doesn't.
  - Aghora tells Sans-Nom his name (or doesn't; some playthroughs end the slice without this).
- **What the slice proves:**
  1. Narrator voice holds across 4-hour playthroughs.
  2. Aghora-as-actor stays in canonical voice across 50+ player interactions.
  3. Tonto's lore stories are interesting enough that players ask for them.
  4. Free-text input doesn't destabilize.
  5. Honcho directional representations produce the right epistemic asymmetry (Aghora knows things Tonto doesn't; both update independently).
  6. The cost model holds.
  7. Players want to play it again.

If 1–7 hold, Phase 2 is greenlit. If any of them fail, the team iterates on Phase 1 until they don't.

---

## 15. Why This Is the Right Game for This Team

The user's existing assets:
- Eight character lore cards, written with the depth of literary criticism.
- A complete directional relationship matrix encoding refracted knowledge.
- A working agent-orchestration system (Council of Castaka) that demonstrates per-character pressure-prompt design and multi-agent deliberation.
- Deep personal familiarity with the IP, including the willingness to treat it as serious creative material.

The game this implies is not a souls-like. It's the game whose architecture is *exactly* what the user has already built, scaled up and turned into a player-facing experience. The Council of Castaka is, in an architectural sense, a Phase-0 prototype of this game's actor-orchestration layer. The character cards are the actor card seeds. The relationship matrix is the directional-representation seed.

A 4-person team building this is leveraging 80% of work the user has already done. The risk is technical execution and tonal discipline, not material creation.

That's the right shape of project for this team and this access level.

---

## Appendix A — Mapping to Honcho Primitives

| Game concept | Honcho primitive |
|---|---|
| Each character | A peer |
| The dynasty's relationship matrix | Directional peer cards (`set_card(target=...)`) |
| What a character knows about another character | Directional representation, with `observe_others=true` |
| The player's character | Peer `sans-nom` |
| A location-arc | A session |
| A scene within a location-arc | A message in the session |
| A character witnessing an event | A message in a session that character is in |
| A character not witnessing an event | The session that character is *not* in |
| Lore that the Narrator must respect | Narrator peer card + retrieved canonical context |
| The Narrator's persona | Narrator peer card with TRAIT entries |
| Stable evolved fact about the player ("Sans-Nom fears the eastern hall") | Card update, Narrator-triggered or via dreaming |
| Tonto's stories | Generated by Tonto's actor agent reading its own representations |
| The dead speaking through the chest-crest | Phase 3 mechanic: ancestor peers' representations of Sans-Nom become readable to Sans-Nom directly |

## Appendix B — Open questions

1. **Does the Narrator have its own evolving peer card across playthroughs?** I.e., does the Narrator-as-author "remember" being a Narrator across all players' games? Probably no, for privacy and tonal reasons. Each playthrough is a fresh Narrator. But worth deciding explicitly.

2. **Should there be a Narrator commentary track / fourth-wall asides?** The Council of Castaka has the `[Tonto, aside]` pattern. This game could surface that for the Narrator too — meta-commentary on what the Narrator is finding interesting in this player's story. Risky (breaks frame); potentially the most interesting feature.

3. **How do peer cards get reset / archived between playthroughs?** Implementation detail. Probably workspace-per-playthrough with a workspace template that includes the seeded actor cards.

4. **Is there ever an explicit ending screen?** The user said no win condition; only death ends the story. But the *form* of the death-screen is editorial. A blank page? A long Tonto eulogy? A canonical-pattern reflection on the kind of Metabaron Sans-Nom became? Worth designing carefully — it's the player's last impression.

5. **Image generation style-lock — Gimenez-faithful or original?** Faithful is more evocative for fans, riskier for licensing. Original (a new painterly style developed for the game) is safer and gives the project its own identity. Probably original, with Gimenez as one reference among several.

6. **Save & resume across devices.** Honcho's hosted backend probably handles this naturally via account-bound workspaces. Verify in Phase 1.

7. **Multiplayer Sans-Noms.** Could multiple players play different Sans-Noms in the same Honcho workspace and have their stories know about each other? (Two Sans-Noms in different timelines, or two characters in one timeline?) Out of scope for v1 but architecturally interesting; the directional-representation system was literally designed for this.

---

---

## Appendix C — Technical Draft

A consolidated technical card for the lead engineer. Pulls together architecture decisions scattered across Sections 4, 5, 9, and 11 into one reference. This is a **draft** — Phase 0 will refine it. Treat it as the starting picture, not the final wiring diagram.

### C.1 Stack

| Layer | Choice | Rationale |
|---|---|---|
| Backend language | **Go 1.22+** | Reuses Council of Castaka patterns (prompt assembly, TUI compositor). Goroutines fit the actor-fanout model cleanly. Single-binary deployment. |
| Memory service | **Honcho via Python sidecar** | Official Python SDK is first-class; no Go SDK exists. Run as `uv`-managed FastAPI service; Go talks to it over local HTTP. |
| LLM provider | **Anthropic** primary | Opus for Narrator, Sonnet for actors. Single-vendor billing/observability. OpenRouter as fallback for outage. |
| Embeddings / lore retrieval | Anthropic + a vector store (probably **Qdrant** or **pgvector**) | Lore corpus is finite and pre-embedded once. No live indexing needed. |
| CLI testbed (Phase 0) | **Bubble Tea** (`charmbracelet/bubbletea`) | Council of Castaka already uses it. Region-based compositor in `internal/ui/render/render.go` is reusable. |
| Web frontend (Phase 1) | **React or Svelte** + Tailwind | Designer's preference. Talks to Go backend over WebSocket for streaming Narrator output. |
| Image gen (Phase 2) | **Flux Pro via Replicate** or **Together** | Routed through the Python sidecar — image gen has better DX in Python than Go. |
| TTS (Phase 3) | **ElevenLabs** | Routed through the Python sidecar for the same reason. |
| Deployment | Single Docker image containing Go binary + Python sidecar; Honcho hosted | Simpler than splitting infra. Honcho self-host is a Phase 3 escape hatch if cost forces it. |

### C.2 Repository structure (Go monorepo)

```
sans-nom/
├── cmd/
│   ├── cli/              # Phase 0 Bubble Tea testbed
│   └── server/           # Phase 1+ web backend
├── internal/
│   ├── narrator/         # Narrator agent: peer card, scene-build loop
│   ├── actors/           # Actor agent registry, per-character invocation
│   ├── scene/            # Scene state, choice generation, free-text resolution
│   ├── lore/             # Canon retrieval — vector search over character cards + comic transcripts
│   ├── bridge/           # HTTP client for the Python sidecar (Honcho + image gen + TTS)
│   ├── prompt/           # Prompt assembly — adapted from council-of-castaka/internal/council/prompt.go
│   └── ui/               # Bubble Tea components — adapted from council-of-castaka/internal/ui
├── bridge/               # Python sidecar (uv-managed)
│   ├── pyproject.toml
│   ├── src/bridge/
│   │   ├── main.py       # FastAPI app
│   │   ├── honcho_ops.py # SDK wrapper exposing the 6 endpoints
│   │   ├── images.py     # Phase 2
│   │   └── tts.py        # Phase 3
│   └── tests/
├── lore/                 # Source of truth — copied/symlinked from council-of-castaka
│   ├── characters/       # The 8 character cards verbatim
│   ├── relations/        # The directional relationship matrix
│   └── canon/            # Comic page transcripts (extracted text + page refs)
├── scripts/
│   ├── seed-honcho.go    # One-shot: bootstrap workspace with seeded peer cards
│   └── eval-narrator.go  # Run the Narrator against the test-scene corpus
├── testdata/
│   └── scenes/           # Phase 0 hand-authored test scenes (the regression set)
├── Makefile              # `make dev` boots both Go and Python
├── docker-compose.yml    # Local dev: Go + Python + (optional) Qdrant
└── README.md
```

### C.3 The Python sidecar API

Six endpoints. Whole service is ~200 lines of FastAPI + Pydantic.

```
POST   /v1/scene/context         → fetch context bundle for Narrator (representations + recent scenes)
POST   /v1/scene/commit          → write scene messages, trigger directional updates
GET    /v1/peer/{id}/card        → fetch peer card (with optional ?target=)
PATCH  /v1/peer/{id}/card        → append/replace card facts (with optional ?target=)
POST   /v1/repr                  → directional representation query (observer + observed + search)
GET    /v1/health                → liveness
```

Local-only by default (binds `127.0.0.1`). Uses Unix socket if you want to be paranoid about `localhost` exposure. Auth is a shared secret in env var; not internet-facing.

### C.4 The agent loop, in pseudocode

```go
// One turn of the game.
func (g *Game) Turn(ctx context.Context, input PlayerInput) (Scene, error) {
    // 1. Assemble Narrator context.
    bundle, err := g.bridge.SceneContext(ctx, SceneContextRequest{
        SessionID:        g.currentSession,
        PlayerPeerID:     "sans-nom",
        RecentSceneCount: 3,
        IncludeReprFor:   []string{"sans-nom", "narrator"},
    })
    // 2. Lore retrieval.
    loreSnippets := g.lore.Search(ctx, input.Text, k: 5)
    
    // 3. Decide which actors are present.
    presentActors := g.scene.PresentActors(g.currentSession)
    
    // 4. Fan out: query each actor in parallel.
    actorOutputs := make(chan ActorOutput, len(presentActors))
    for _, actor := range presentActors {
        go func(a Actor) {
            repr, _ := g.bridge.GetRepresentation(ctx, a.ID, target: "sans-nom")
            output := g.actors.Query(ctx, a, ActorQuery{
                SceneSetup:   bundle.LastNarratorBeat,
                MyRepr:       repr,
                PlayerInput:  input,
            })
            actorOutputs <- output
        }(actor)
    }
    outputs := collect(actorOutputs, len(presentActors))
    
    // 5. Narrator synthesizes.
    scene, err := g.narrator.Compose(ctx, NarratorInput{
        Bundle:         bundle,
        LoreSnippets:   loreSnippets,
        ActorOutputs:   outputs,
        PlayerInput:    input,
    })
    
    // 6. Commit to Honcho.
    g.bridge.CommitScene(ctx, CommitRequest{
        SessionID:   g.currentSession,
        Scene:       scene,
        Witnesses:   presentActors,
    })
    
    return scene, nil
}
```

Concrete LOC estimate for the loop itself: ~400 Go lines. The character cards + lore + prompt scaffolding are existing assets; this is glue + orchestration.

### C.5 Honcho session granularity (decision needed in Phase 0)

The indie GDD specifies "one session per location-arc." This is the right shape but Phase 0 needs to decide concretely:

- **Where does an arc start/end?** Spatial change (Sans-Nom leaves the compound) is the obvious cue. Temporal jump (years pass via "Wait") is the other. Combined predicate.
- **What happens at session boundaries to representations?** New session inherits the peer cards (they're per-peer, session-independent), but the per-session message history starts fresh. The Narrator must explicitly load summary context from the prior session.
- **How does Tonto-as-storyteller work across sessions?** Tonto's representations of long-dead characters span all sessions. When Tonto tells a story in session N, his actor agent reads its full historical representation, not just the current session. This is automatic in Honcho's default behavior.

These are runtime-policy choices, not architecture choices — the architecture supports any of them.

### C.6 Cost discipline

Phase 0 burn rate (4 devs hand-playtesting):
- ~50 turns/dev/day × 4 devs × $0.07/turn = **$14/day** during active dev
- Negligible against unlimited-AI access budget.

Phase 1 beta (closed cohort, ~50 testers):
- Each runs a ~4hr playthrough averaged 2x = ~1,440 turns × 50 testers = ~72k turns × $0.07 = **~$5,000 over the beta window**.
- Real money. Bound it. Use cohort sign-up + per-tester turn cap.

Phase 3 production:
- See indie GDD §9 monetization paths. BYOK is the structural escape hatch if subscription pricing doesn't pencil.

### C.7 Observability — what to instrument from day one

The agent loop has more failure modes than a normal product, and most of them are silent (Narrator drift, actor-bleed, lore-retrieval misses). Instrument these from the first commit:

- **Per-turn token usage** by agent (Narrator vs. each actor).
- **Per-turn latency** broken into Narrator/actor-fanout/Honcho/lore-retrieval.
- **Lore-retrieval hit/miss/score distribution.** A cold hit rate above 30% means the corpus or the embedding strategy needs work.
- **Scene-length distribution.** Narrator drift toward generic sci-fi often shows up as scenes getting longer or shorter than the established baseline.
- **Card-update events.** Every `set_card` call logged with diff. Lets you replay the dynasty's evolution.
- **Player free-text vs. choice-pick ratio.** Tells you whether the choice-list is doing its job or being ignored.

Trace IDs propagate Go → Python sidecar → Honcho. OpenTelemetry is the path of least resistance.

### C.8 Open technical questions for Phase 0

These are the things that need to be decided early because they're load-bearing:

1. **Streaming vs. blocking Narrator output to the player.** Streaming feels better but complicates the commit-to-Honcho flow (you commit only the final text, not the streamed tokens). Recommendation: stream to player UI, commit committed-text post-stream.
2. **Replay determinism.** Is a player's playthrough deterministic given the same RNG seed + Honcho state? Probably no (LLM non-determinism), but you might want a *replayable* mode for QA. Decide in Phase 0.
3. **Schema for committed scene text.** Markdown? Structured JSON with a `prose` field plus metadata? Recommend structured-with-prose-as-a-field — opens the door to scene-level annotations later (mood, tempo, character interiority surfaces).
4. **Card-eviction policy.** Honcho's 40-fact-per-card cap will bite eventually. Need a strategy: most-recent? Most-referenced? Narrator-curated? Probably Narrator-curated with a fallback to most-recent.
5. **The `narrator` peer's existence.** Treat the Narrator as a Honcho peer (so it has its own representations of all characters) or as pure stateless authoring? Recommendation: Narrator-as-peer in Phase 1 — gives you the audit pass capability for free.

---

*End of document.*

*This GDD assumes the comics-to-game-spec skill workflow as foundation, with rescoping for indie team + AI-generative-access constraints. It supersedes — for this team and access level — the 80-person souls-like spec; the souls-like spec remains the right design at AAA scale.*
