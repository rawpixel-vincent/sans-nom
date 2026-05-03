# Death State

How a playthrough ends. Sans-Nom can die. When he does, the game does not restart — it concludes. The workspace is archived; the player is given an ending that feels earned and specific to *this* boy.

This is a Phase 2 deliverable. Phase 1 does not include death. See [Phase 2 scope](../../phases/phase-2-adolescence/scope.md).

## Design intent

Death is not failure. It is one of the possible shapes of a Sans-Nom's life. Some boys die young, in violence. Some die old, before the Bushitaka decision lands. Some die having refused everything the dynasty asked. All are valid.

The death state must:
- Feel specific to *this* playthrough. A generic "you died" screen breaks the IP's grammar.
- Give Tonto the last word. He is the dynasty's narrator; ending without him would be wrong.
- Make the player feel their Sans-Nom mattered — even if the dynasty did not mourn him.
- Leave the door open to a new boy without diminishing the one who ended.

## The death event sequence

### 1. The death scene

The Narrator writes the death as a scene — the moment itself, whatever it is. Sans-Nom dies in prose. The scene does not cut away; it finishes.

The Narrator must write the death in tone: not melodramatic, not clinical. The IP's register is *operatic austerity* — grand in scope, spare in language. A single well-chosen image is worth three paragraphs of grief.

### 2. The Tonto eulogy

After the death scene, Tonto speaks. His actor agent is invoked with a specific prompt: *"Sans-Nom is dead. Write a Tonto-style eulogy — long, digressive, specific to what Tonto witnessed of this boy across his life. Begin with an image. End with the boy's place in the dynasty."*

The eulogy draws on Tonto's full representation of Sans-Nom (accumulated across the entire playthrough). This is what makes it specific: Tonto can only say what he actually witnessed. A Sans-Nom who never asked Tonto questions will have a shorter eulogy than one who came to the alcove often. A Sans-Nom who refused training will have a different eulogy than one who excelled at it.

The eulogy is long by design — typically 8–15 minutes of reading. The player is given the ending they earned.

### 3. The archive naming

After the eulogy, the workspace is named. The default format:

> *Sans-Nom who [defining arc in two or three words], ages 6–[death age]*

Examples:
- *Sans-Nom who opened the hall, ages 6–14*
- *Sans-Nom who refused the name, ages 6–19*
- *Sans-Nom who trained in silence, ages 6–11*

The naming is Narrator-generated from the playthrough's dominant card facts. The player sees the name as the workspace closes and it enters the "boys who lived" archive.

### 4. The workspace archival

The workspace closes. It is not deleted. It enters the "boys who lived" view — accessible via [Recall](../verbs/recall.md); the player can re-read the story of their dead boy indefinitely.

A single affordance is offered on the closing screen: **"Begin again."** No other prompt. The new boy will have no memory of his predecessor.

## The closing screen

A single page. No UI chrome. The workspace name, centered. Below it, a one-line Tonto epitaph — the last line of the eulogy, or a shorter formulation generated specifically for the screen. Below that: **"Begin again."**

The screen does not explain what happened. The player was there.

## What the death state must never do

- Summarize the playthrough as a series of achievements. There are no achievements.
- Imply the player made a mistake. The dynasty does not grade its children.
- Rush. The eulogy is long. Players who do not want to read can scroll. The option to rush exists; the default should not be speed.
- Kill the workspace's accessibility. Dead boys' stories are permanent.

## Causes of death

The Narrator decides when Sans-Nom dies and how, based on scene logic. There is no health bar. There is no explicit death trigger. Some ways a Sans-Nom might die:

- Violence (training gone wrong; enemy encounter; the Bushitaka decision turned fatal).
- Old age or illness (a playthrough that runs long enough).
- Self-destruction (a Sans-Nom who refused everything and found no other shape).
- Narrative conclusion the Narrator judges complete. (The player has reached a natural endpoint; the Narrator ends it with grace rather than extending indefinitely.)

The player cannot force a death, but they can approach it. A Sans-Nom who consistently chooses danger, refuses training, and walks into the eastern hall without preparation is building toward something. The Narrator is the one who decides when that building is complete.

## Relationship to the Bushitaka decision

Phase 3's pivotal moment — the Bushitaka kill-or-refuse — is not the same as death state. A Sans-Nom can die before the decision. A Sans-Nom can survive the decision (having performed the kill or found some other resolution). Death state is always available; the Bushitaka decision is phase-gated.

A Sans-Nom who dies *at* the Bushitaka decision — neither completing nor refusing it — is a valid outcome. Tonto's eulogy in that case carries the weight of the unfinished.

## Implementation notes

- Death events emit a `workspace.death` telemetry event with: death age, cause category (violence / natural / narrative), session count, scene count.
- The eulogy generation is a single large actor call (Tonto, full representation, full context bundle). Cost is higher than a normal turn — acceptable for a once-per-playthrough event. See [per-turn cost model](../../project/cost/per-turn-model.md).
- The "boys who lived" archive view is a Phase 2 UI deliverable. See [Phase 2 scope](../../phases/phase-2-adolescence/scope.md).
