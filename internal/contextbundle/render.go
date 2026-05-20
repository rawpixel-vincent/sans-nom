package contextbundle

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func (c NarratorContext) RenderText() string {
	var b strings.Builder
	writeSection(&b, "ROLE BOUNDARY")
	fmt.Fprintf(&b, "You are the Narrator. Turn only the selected scene node into player-facing prose.\n")
	fmt.Fprintf(&b, "Lead with scene, character action, sensory detail, and emotional movement; let law, technology, and taboo appear through lived consequence.\n")
	fmt.Fprintf(&b, "Do not expose Writer drafts, unused branches, private actor interiors, or planning notes.\n")
	fmt.Fprintf(&b, "Honor the lorepack scenario contract and use lore references as fidelity support for this active scenario.\n")

	writeStoryBibleDoctrine(&b, c.StoryBible)
	writeScenarioContract(&b, c.ScenarioContract)

	writeSection(&b, "CURRENT SCENE STATE")
	writeJSON(&b, c.SceneState)

	writeSection(&b, "PLAYER DECISION")
	fmt.Fprintln(&b, decisionText(c.PlayerDecision))

	writeSection(&b, "SELECTED NODE")
	writeJSON(&b, c.SelectedNode)

	writeActorContexts(&b, c.ActorContexts)
	writeLoreGroups(&b, groupLoreByKind(c.LoreHits))

	writeStringList(&b, "RECENT WITNESSED CANON", c.RecentScenes)
	writeRepresentations(&b, "DIRECTIONAL REPRESENTATIONS", c.Representations)
	writeStringList(&b, "MEMORY AND WITNESS RULES", c.WitnessRules)
	writeStringList(&b, "WARNINGS", c.Warnings)

	return capRenderedText(strings.TrimSpace(b.String()), c.MaxRenderChars)
}

func (c NarratorContext) RenderActorText() string {
	var b strings.Builder
	writeSection(&b, "ROLE BOUNDARY")
	fmt.Fprintln(&b, "You are acting inside the current scene only. Use the character card, current scene beats, player decision, and provided directional representations as your available knowledge.")

	writeSection(&b, "CURRENT SCENE STATE")
	writeJSON(&b, c.SceneState)

	writeSection(&b, "PLAYER DECISION")
	fmt.Fprintln(&b, decisionText(c.PlayerDecision))

	writeSection(&b, "SELECTED NODE")
	writeJSON(&b, c.SelectedNode)

	writeActorContexts(&b, c.ActorContexts)
	writeStringList(&b, "MEMORY AND WITNESS RULES", c.WitnessRules)
	writeStringList(&b, "WARNINGS", c.Warnings)

	return capRenderedText(strings.TrimSpace(b.String()), c.MaxRenderChars)
}

func (c WriterContext) RenderText() string {
	var b strings.Builder
	writeSection(&b, "ROLE BOUNDARY")
	fmt.Fprintf(&b, "You are the Creative Writer. Plan the next bounded packet from the latest witnessed canon and player decision; the packet tree is planning material, not canon.\n")
	fmt.Fprintf(&b, "Focus on immediate consequence, next playable pressure, character desire, opposition, relationship stakes, and scene movement.\n")
	fmt.Fprintf(&b, "The Narrator owns final prose. Honcho receives only rolled-out witnessed canon, never unused branches.\n")
	fmt.Fprintf(&b, "Honor the lorepack scenario contract; branch inside its pressure, choice logic, and consequences.\n")

	writeStoryBibleDoctrine(&b, c.StoryBible)
	writeScenarioContract(&b, c.ScenarioContract)

	writeSection(&b, "CURRENT SCENE STATE")
	writeJSON(&b, c.SceneState)

	writeSection(&b, "LATEST ROLLOUT AND DECISION")
	fmt.Fprintln(&b, decisionText(c.PlayerDecision))

	writeWriterSelectedNodeSummary(&b, c)

	writeStringList(&b, "RECENT WITNESSED CANON", c.RecentScenes)
	writeWriterProgressionContract(&b)
	writeRepresentations(&b, "DIRECTIONAL REPRESENTATIONS", c.Representations)
	writeLoreGroups(&b, c.LoreByKind)
	writeActorManifest(&b, c.ActorManifest)

	writeSection(&b, "PLANNING CONSTRAINTS")
	fmt.Fprintln(&b, "- Revise only the current bounded packet.")
	fmt.Fprintln(&b, "- StoryPacket closure rule: every choice target_node_id must point to a node in this same packet; future packet handoffs must be terminal nodes with revision_notes, not missing target ids.")
	fmt.Fprintln(&b, "- Keep all branches outside Honcho until the Narrator rolls one out.")
	fmt.Fprintln(&b, "- Preserve the scenario contract's must_honor items and avoid must_not_violate drift.")
	fmt.Fprintln(&b, "- Make each major choice express the scenario choice_principles.")
	fmt.Fprintln(&b, "- Let binding doctrine sharpen desire, opposition, or cost inside the player character's lived order.")
	fmt.Fprintln(&b, "- Make strange material feel lived-in through internal cause, social practice, emotional credibility, visible cost, and consequence.")
	fmt.Fprintln(&b, "- Actor agents are available only for ids listed in STORY ACTOR MANIFEST; otherwise keep actor_needs empty and write behavior as scene beats.")
	fmt.Fprintln(&b, "- Use actor cards and directional representations only for story actors explicitly present in the packet.")
	fmt.Fprintln(&b, "- Treat lore references as support for the active scenario, not as automatic all-cast inventory.")

	writeStringList(&b, "WARNINGS", c.Warnings)

	writeSection(&b, "CURRENT BOUNDED PACKET REFERENCE")
	fmt.Fprintln(&b, "The full packet JSON below is prior planning context. The selected node and recent witnessed canon above have priority when they disagree or when this section is truncated.")
	writeJSON(&b, c.Packet)

	return capRenderedText(strings.TrimSpace(b.String()), c.MaxRenderChars)
}

func writeWriterProgressionContract(b *strings.Builder) {
	writeSection(b, "STORY PROGRESSION CONTRACT")
	fmt.Fprintln(b, "- Start after the selected node's witnessed outcome; do not replay, re-adjudicate, or rename the same pending proof.")
	fmt.Fprintln(b, "- Each refresh packet must advance at least one macro story state: location, custody/control, revealed truth, opponent advantage, resource/weapon status, witness record, or irreversible cost.")
	fmt.Fprintln(b, "- If the previous branch accepted a challenge, petition, escape, wound, testimony, or bargain, the next packet must make that consequence happen or move to the scene where it bites.")
	fmt.Fprintln(b, "- Do not offer three choices that are only different ways to continue observing the same mechanism. Each choice must change who can act, what proof exists, what relationship is endangered, or what cost becomes unavoidable.")
	fmt.Fprintln(b, "- Use the Story Bible story_arc, if present, as the phase ladder. The opening crisis is not the whole story.")
}

func writeWriterSelectedNodeSummary(b *strings.Builder, c WriterContext) {
	writeSection(b, "SELECTED NODE")
	fmt.Fprintf(b, "selected_node_id: %s\n", c.SelectedNodeID)
	node, err := c.Packet.Node(c.SelectedNodeID)
	if err != nil {
		return
	}
	fmt.Fprintf(b, "title: %s\n", node.Title)
	if len(node.Choices) > 0 {
		fmt.Fprintln(b, "choices:")
		for _, choice := range node.Choices {
			target := strings.TrimSpace(choice.TargetNodeID)
			if target != "" {
				fmt.Fprintf(b, "- %s -> %s: %s\n", choice.ID, target, choice.Label)
			} else {
				fmt.Fprintf(b, "- %s: %s\n", choice.ID, choice.Label)
			}
		}
	}
	writeInlineList(b, "revision_notes", node.RevisionNotes)
	writeInlineList(b, "invalidation_notes", node.InvalidationNotes)
	if strings.TrimSpace(node.WriterNotes) != "" {
		fmt.Fprintf(b, "writer_notes: %s\n", strings.TrimSpace(node.WriterNotes))
	}
}

func writeScenarioContract(b *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	writeSection(b, "LOREPACK SCENARIO CONTRACT")
	fmt.Fprintln(b, value)
}

func writeStoryBibleDoctrine(b *strings.Builder, value *story.StoryBible) {
	if value == nil {
		return
	}
	writeSection(b, "STORY BIBLE DOCTRINE")
	fmt.Fprintf(b, "title: %s\n", value.Title)
	fmt.Fprintf(b, "premise: %s\n", value.Premise)
	fmt.Fprintf(b, "player: %s (%s)\n", value.PlayerCharacter.Name, value.PlayerCharacter.PeerID)
	fmt.Fprintf(b, "player_world_connection: %s\n", value.PlayerWorldConnection)
	for _, law := range value.BindingLorePrinciples() {
		fmt.Fprintf(b, "law %s (%s): %s\n", law.ID, law.Name, law.Description)
		fmt.Fprintf(b, "enforcement: %s\n", law.Enforcement)
		fmt.Fprintf(b, "why_profound: %s\n", law.WhyProfound)
		fmt.Fprintf(b, "not_merely_cruel: %s\n", law.NotMerelyCruel)
	}
	writeInlineList(b, "world_basics", value.WorldBasics)
	writeInlineList(b, "tone_doctrine", value.ToneDoctrine)
	writeInlineList(b, "first_turn_exposition", value.FirstTurnExposition)
	writeInlineList(b, "story_arc", value.StoryArc)
	if value.ResumeSummary != "" {
		fmt.Fprintf(b, "resume_summary: %s\n", value.ResumeSummary)
	}
}

func writeInlineList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(b, "- %s\n", strings.TrimSpace(value))
		}
	}
}

func writeSection(b *strings.Builder, title string) {
	if b.Len() > 0 {
		fmt.Fprintln(b)
		fmt.Fprintln(b)
	}
	fmt.Fprintf(b, "## %s\n", title)
}

func writeJSON(b *strings.Builder, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(b, "%v\n", value)
		return
	}
	fmt.Fprintln(b, string(data))
}

func writeStringList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	writeSection(b, title)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func writeRepresentations(b *strings.Builder, title string, reps []Representation) {
	if len(reps) == 0 {
		return
	}
	reps = append([]Representation(nil), reps...)
	sort.SliceStable(reps, func(i, j int) bool {
		if reps[i].Observer == reps[j].Observer {
			return reps[i].Observed < reps[j].Observed
		}
		return reps[i].Observer < reps[j].Observer
	})
	writeSection(b, title)
	for _, rep := range reps {
		fmt.Fprintf(b, "### %s -> %s\n%s\n", rep.Observer, rep.Observed, strings.TrimSpace(rep.Text))
	}
}

func writeActorContexts(b *strings.Builder, actors []ActorContext) {
	if len(actors) == 0 {
		return
	}
	actors = append([]ActorContext(nil), actors...)
	sort.SliceStable(actors, func(i, j int) bool {
		if actors[i].ActorID == actors[j].ActorID {
			return actors[i].Mode < actors[j].Mode
		}
		return actors[i].ActorID < actors[j].ActorID
	})
	writeSection(b, "ACTOR CONTEXTS")
	for _, actor := range actors {
		fmt.Fprintf(b, "### %s\n", actor.ActorID)
		fmt.Fprintf(b, "mode: %s\n", actor.Mode)
		if actor.Reason != "" {
			fmt.Fprintf(b, "reason: %s\n", actor.Reason)
		}
		if len(actor.Targets) > 0 {
			fmt.Fprintf(b, "targets: %s\n", strings.Join(actor.Targets, ", "))
		}
		fmt.Fprintf(b, "card_source: %s\n", actor.CardSource)
		fmt.Fprintln(b, "card:")
		for _, line := range strings.Split(actor.Card, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Fprintf(b, "- %s\n", line)
			}
		}
		writeInlineRepresentations(b, actor.Representations)
		writeInlineLore(b, actor.LoreHits)
		if len(actor.Warnings) > 0 {
			fmt.Fprintln(b, "warnings:")
			for _, warning := range actor.Warnings {
				fmt.Fprintf(b, "- %s\n", warning)
			}
		}
	}
}

func writeInlineRepresentations(b *strings.Builder, reps []Representation) {
	if len(reps) == 0 {
		return
	}
	reps = append([]Representation(nil), reps...)
	sort.SliceStable(reps, func(i, j int) bool {
		if reps[i].Observer == reps[j].Observer {
			return reps[i].Observed < reps[j].Observed
		}
		return reps[i].Observer < reps[j].Observer
	})
	fmt.Fprintln(b, "representations:")
	for _, rep := range reps {
		fmt.Fprintf(b, "- %s -> %s: %s\n", rep.Observer, rep.Observed, strings.TrimSpace(rep.Text))
	}
}

func writeInlineLore(b *strings.Builder, hits []LoreHit) {
	if len(hits) == 0 {
		return
	}
	hits = append([]LoreHit(nil), hits...)
	sortLore(hits)
	fmt.Fprintln(b, "scene-valid lore:")
	for _, hit := range hits {
		fmt.Fprintf(b, "- %.3f %s %s: %s\n", hit.Score, hit.ID, hit.Source, strings.TrimSpace(hit.Text))
	}
}

func writeLoreGroups(b *strings.Builder, byKind map[string][]LoreHit) {
	if len(byKind) == 0 {
		return
	}
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	writeSection(b, "LORE FIDELITY REFERENCES")
	for _, kind := range kinds {
		hits := append([]LoreHit(nil), byKind[kind]...)
		sortLore(hits)
		fmt.Fprintf(b, "### %s\n", kind)
		for _, hit := range hits {
			fmt.Fprintf(b, "- %.3f %s %s\n", hit.Score, hit.ID, hit.Source)
			if text := strings.TrimSpace(hit.Text); text != "" {
				fmt.Fprintf(b, "  %s\n", text)
			}
		}
	}
}

func writeActorManifest(b *strings.Builder, actors []ActorManifestItem) {
	if len(actors) == 0 {
		return
	}
	actors = append([]ActorManifestItem(nil), actors...)
	sort.SliceStable(actors, func(i, j int) bool {
		if actors[i].Explicit != actors[j].Explicit {
			return actors[i].Explicit
		}
		return actors[i].PeerID < actors[j].PeerID
	})
	writeSection(b, "STORY ACTOR MANIFEST")
	for _, actor := range actors {
		marker := ""
		if actor.Explicit {
			marker = " explicit"
		}
		fmt.Fprintf(b, "- %s%s", actor.PeerID, marker)
		if actor.DisplayName != "" {
			fmt.Fprintf(b, " (%s)", actor.DisplayName)
		}
		if actor.Role != "" {
			fmt.Fprintf(b, ": %s", actor.Role)
		}
		if actor.Reason != "" {
			fmt.Fprintf(b, " [%s]", actor.Reason)
		}
		fmt.Fprintln(b)
	}
}

func capRenderedText(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	notice := "\n\n[context truncated at render cap]"
	if max <= len(notice) {
		return text[:max]
	}
	return strings.TrimSpace(text[:max-len(notice)]) + notice
}
