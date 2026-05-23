package contextbundle

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/modelcap"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func (c NarratorContext) RenderText() string {
	return renderSectionTexts(c.narratorSections())
}

func (c NarratorContext) RenderBudgetedText(budget modelcap.Budget) (string, RenderBudgetReport) {
	return renderBudgetedSections("narrator", c.narratorSections(), budget)
}

func (c NarratorContext) RenderActorText() string {
	return renderSectionTexts(c.actorSections())
}

func (c WriterContext) RenderText() string {
	return renderSectionTexts(c.writerSections())
}

func (c WriterContext) RenderBudgetedText(budget modelcap.Budget) (string, RenderBudgetReport) {
	return renderBudgetedSections("writer", c.writerSections(), budget)
}

type contextSection struct {
	ID        string
	Title     string
	Text      string
	Required  bool
	Priority  int
	MinTokens int
}

func (c NarratorContext) narratorSections() []contextSection {
	return nonEmptySections([]contextSection{
		renderedSection("role_boundary", "ROLE BOUNDARY", true, 100, 500, func(b *strings.Builder) {
			fmt.Fprintf(b, "You are the Narrator. Turn only the selected scene node into player-facing prose.\n")
			fmt.Fprintf(b, "This is a game: keep the player character centered, make the scene entertaining to play, and aim pressure at player agency.\n")
			fmt.Fprintf(b, "Lead with scene, character action, sensory detail, and emotional movement; let law, technology, and taboo appear through lived consequence.\n")
			fmt.Fprintf(b, "Do not expose Writer drafts, unused branches, private actor interiors, or planning notes.\n")
			fmt.Fprintf(b, "Honor the lorepack scenario contract and use lore references as fidelity support for this active scenario.\n")
		}),
		renderedSection("current_scene_state", "CURRENT SCENE STATE", true, 100, 700, func(b *strings.Builder) {
			writeJSON(b, c.SceneState)
		}),
		renderedSection("player_decision", "PLAYER DECISION", true, 100, 400, func(b *strings.Builder) {
			fmt.Fprintln(b, decisionText(c.PlayerDecision))
		}),
		renderedSection("selected_node", "SELECTED NODE", true, 100, 1400, func(b *strings.Builder) {
			writeJSON(b, c.SelectedNode)
		}),
		renderedSection("actor_contexts", "ACTOR CONTEXTS", true, 90, 1200, func(b *strings.Builder) {
			writeActorContextsBody(b, c.ActorContexts)
		}),
		renderedSection("recent_witnessed_canon", "RECENT WITNESSED CANON", true, 90, 1000, func(b *strings.Builder) {
			writeStringListBody(b, c.RecentScenes)
		}),
		renderedSection("directional_representations", "DIRECTIONAL REPRESENTATIONS", false, 40, 800, func(b *strings.Builder) {
			writeRepresentationsBody(b, c.Representations)
		}),
		renderedSection("story_bible_doctrine", "STORY BIBLE DOCTRINE", false, 30, 1200, func(b *strings.Builder) {
			writeStoryBibleDoctrineBody(b, c.StoryBible)
		}),
		renderedSection("scenario_contract", "LOREPACK SCENARIO CONTRACT", false, 35, 1200, func(b *strings.Builder) {
			writeScenarioContractBody(b, c.ScenarioContract)
		}),
		renderedSection("lore_references", "LORE FIDELITY REFERENCES", false, 20, 800, func(b *strings.Builder) {
			writeLoreGroupsBody(b, groupLoreByKind(c.LoreHits))
		}),
		renderedSection("witness_rules", "MEMORY AND WITNESS RULES", true, 100, 700, func(b *strings.Builder) {
			writeStringListBody(b, c.WitnessRules)
		}),
		renderedSection("warnings", "WARNINGS", false, 10, 300, func(b *strings.Builder) {
			writeStringListBody(b, c.Warnings)
		}),
	})
}

func (c NarratorContext) actorSections() []contextSection {
	return nonEmptySections([]contextSection{
		renderedSection("role_boundary", "ROLE BOUNDARY", true, 100, 500, func(b *strings.Builder) {
			fmt.Fprintln(b, "You are acting inside the current scene only. Use the character card, current scene beats, player decision, and provided directional representations as your available knowledge.")
		}),
		renderedSection("current_scene_state", "CURRENT SCENE STATE", true, 100, 700, func(b *strings.Builder) {
			writeJSON(b, c.SceneState)
		}),
		renderedSection("player_decision", "PLAYER DECISION", true, 100, 400, func(b *strings.Builder) {
			fmt.Fprintln(b, decisionText(c.PlayerDecision))
		}),
		renderedSection("selected_node", "SELECTED NODE", true, 100, 1400, func(b *strings.Builder) {
			writeJSON(b, c.SelectedNode)
		}),
		renderedSection("actor_contexts", "ACTOR CONTEXTS", true, 100, 1200, func(b *strings.Builder) {
			writeActorContextsBody(b, c.ActorContexts)
		}),
		renderedSection("witness_rules", "MEMORY AND WITNESS RULES", true, 100, 700, func(b *strings.Builder) {
			writeStringListBody(b, c.WitnessRules)
		}),
		renderedSection("warnings", "WARNINGS", false, 10, 300, func(b *strings.Builder) {
			writeStringListBody(b, c.Warnings)
		}),
	})
}

func (c WriterContext) writerSections() []contextSection {
	return nonEmptySections([]contextSection{
		renderedSection("role_boundary", "ROLE BOUNDARY", true, 100, 600, func(b *strings.Builder) {
			fmt.Fprintf(b, "You are the Creative Writer. Plan the next bounded packet from the latest witnessed canon and player decision; the packet tree is planning material, not canon.\n")
			fmt.Fprintf(b, "This is a game: keep the player character at the dramatic center, make the next pressure entertaining to play, and give the player agency rather than only lore procedure.\n")
			fmt.Fprintf(b, "Focus on immediate consequence, next playable pressure, character desire, opposition, relationship stakes, and scene movement.\n")
			fmt.Fprintf(b, "The Narrator owns final prose. Honcho receives only rolled-out witnessed canon, never unused branches.\n")
			fmt.Fprintf(b, "Honor the lorepack scenario contract; branch inside its pressure, choice logic, and consequences.\n")
		}),
		renderedSection("story_bible_doctrine", "STORY BIBLE DOCTRINE", false, 30, 1400, func(b *strings.Builder) {
			writeStoryBibleDoctrineBody(b, c.StoryBible)
		}),
		renderedSection("scenario_contract", "LOREPACK SCENARIO CONTRACT", false, 35, 1400, func(b *strings.Builder) {
			writeScenarioContractBody(b, c.ScenarioContract)
		}),
		renderedSection("current_scene_state", "CURRENT SCENE STATE", true, 100, 700, func(b *strings.Builder) {
			writeJSON(b, c.SceneState)
		}),
		renderedSection("latest_rollout_decision", "LATEST ROLLOUT AND DECISION", true, 100, 500, func(b *strings.Builder) {
			fmt.Fprintln(b, decisionText(c.PlayerDecision))
		}),
		renderedSection("selected_node", "SELECTED NODE", true, 100, 1200, func(b *strings.Builder) {
			writeWriterSelectedNodeSummaryBody(b, c)
		}),
		renderedSection("story_phase_guidance", "STORY PHASE GUIDANCE", true, 100, 1200, func(b *strings.Builder) {
			writeStoryProgressionBody(b, c.Progression)
		}),
		renderedSection("actor_manifest", "STORY ACTOR MANIFEST", true, 95, 1000, func(b *strings.Builder) {
			writeActorManifestBody(b, c.ActorManifest)
		}),
		renderedSection("recent_witnessed_canon", "RECENT WITNESSED CANON", true, 95, 1000, func(b *strings.Builder) {
			writeStringListBody(b, c.RecentScenes)
		}),
		renderedSection("progression_contract", "STORY PROGRESSION CONTRACT", true, 100, 900, func(b *strings.Builder) {
			writeWriterProgressionContractBody(b)
		}),
		renderedSection("directional_representations", "DIRECTIONAL REPRESENTATIONS", false, 40, 800, func(b *strings.Builder) {
			writeRepresentationsBody(b, c.Representations)
		}),
		renderedSection("lore_references", "LORE FIDELITY REFERENCES", false, 20, 900, func(b *strings.Builder) {
			writeLoreGroupsBody(b, c.LoreByKind)
		}),
		renderedSection("planning_constraints", "PLANNING CONSTRAINTS", true, 100, 1000, func(b *strings.Builder) {
			writePlanningConstraintsBody(b)
		}),
		renderedSection("warnings", "WARNINGS", false, 10, 300, func(b *strings.Builder) {
			writeStringListBody(b, c.Warnings)
		}),
		renderedSection("packet_reference", "CURRENT BOUNDED PACKET REFERENCE", false, 25, 1400, func(b *strings.Builder) {
			fmt.Fprintln(b, "The full packet JSON below is prior planning context. The selected node and recent witnessed canon above have priority when they disagree or when this section is trimmed.")
			writeJSON(b, c.Packet)
		}),
	})
}

func writeWriterProgressionContract(b *strings.Builder) {
	writeSection(b, "STORY PROGRESSION CONTRACT")
	writeWriterProgressionContractBody(b)
}

func writeWriterProgressionContractBody(b *strings.Builder) {
	fmt.Fprintln(b, "- Start after the selected node's witnessed outcome; do not replay, re-adjudicate, or rename the same pending proof.")
	fmt.Fprintln(b, "- Each refresh packet must advance at least one macro story state: location, custody/control, revealed truth, opponent advantage, resource/weapon status, witness record, or irreversible cost.")
	fmt.Fprintln(b, "- If the previous branch accepted a challenge, petition, escape, wound, testimony, or bargain, the next packet must make that consequence happen or move to the scene where it bites.")
	fmt.Fprintln(b, "- Do not offer three choices that are only different ways to continue observing the same mechanism. Each choice must change who can act, what proof exists, what relationship is endangered, or what cost becomes unavoidable.")
	fmt.Fprintln(b, "- Use the Story Bible story_arc, if present, as the phase ladder. The opening crisis is not the whole story.")
}

func writeStoryProgression(b *strings.Builder, progression StoryProgression) {
	if len(progression.StoryArc) == 0 && progression.CurrentPhase == "" && progression.NextPhase == "" && len(progression.SpentMotifs) == 0 && len(progression.RequiredMovement) == 0 && len(progression.Warnings) == 0 {
		return
	}
	writeSection(b, "STORY PHASE GUIDANCE")
	writeStoryProgressionBody(b, progression)
}

func writeStoryProgressionBody(b *strings.Builder, progression StoryProgression) {
	writeInlineList(b, "story_arc", progression.StoryArc)
	if progression.CurrentPhase != "" {
		fmt.Fprintf(b, "current_phase: %s\n", progression.CurrentPhase)
	}
	if progression.NextPhase != "" {
		fmt.Fprintf(b, "next_phase: %s\n", progression.NextPhase)
	}
	writeInlineList(b, "spent_motifs", progression.SpentMotifs)
	writeInlineList(b, "required_next_movement", progression.RequiredMovement)
	writeInlineList(b, "phase_warnings", progression.Warnings)
}

func writeWriterSelectedNodeSummary(b *strings.Builder, c WriterContext) {
	writeSection(b, "SELECTED NODE")
	writeWriterSelectedNodeSummaryBody(b, c)
}

func writeWriterSelectedNodeSummaryBody(b *strings.Builder, c WriterContext) {
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
	writeScenarioContractBody(b, value)
}

func writeScenarioContractBody(b *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintln(b, value)
}

func writeStoryBibleDoctrine(b *strings.Builder, value *story.StoryBible) {
	if value == nil {
		return
	}
	writeSection(b, "STORY BIBLE DOCTRINE")
	writeStoryBibleDoctrineBody(b, value)
}

func writeStoryBibleDoctrineBody(b *strings.Builder, value *story.StoryBible) {
	if value == nil {
		return
	}
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
	if strings.TrimSpace(value.CurrentArcPhase) != "" {
		fmt.Fprintf(b, "current_arc_phase: %s\n", strings.TrimSpace(value.CurrentArcPhase))
	}
	if strings.TrimSpace(value.NextRequiredRevelation) != "" {
		fmt.Fprintf(b, "next_required_revelation: %s\n", strings.TrimSpace(value.NextRequiredRevelation))
	}
	if strings.TrimSpace(value.NextRequiredOpposition) != "" {
		fmt.Fprintf(b, "next_required_opposition: %s\n", strings.TrimSpace(value.NextRequiredOpposition))
	}
	writeInlineList(b, "spent_motifs", value.SpentMotifs)
	writeInlineList(b, "open_threads", value.OpenThreads)
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
	writeStringListBody(b, values)
}

func writeStringListBody(b *strings.Builder, values []string) {
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
	writeSection(b, title)
	writeRepresentationsBody(b, reps)
}

func writeRepresentationsBody(b *strings.Builder, reps []Representation) {
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
	for _, rep := range reps {
		fmt.Fprintf(b, "### %s -> %s\n%s\n", rep.Observer, rep.Observed, strings.TrimSpace(rep.Text))
	}
}

func writeActorContexts(b *strings.Builder, actors []ActorContext) {
	if len(actors) == 0 {
		return
	}
	writeSection(b, "ACTOR CONTEXTS")
	writeActorContextsBody(b, actors)
}

func writeActorContextsBody(b *strings.Builder, actors []ActorContext) {
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
	writeSection(b, "LORE FIDELITY REFERENCES")
	writeLoreGroupsBody(b, byKind)
}

func writeLoreGroupsBody(b *strings.Builder, byKind map[string][]LoreHit) {
	if len(byKind) == 0 {
		return
	}
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
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
	writeSection(b, "STORY ACTOR MANIFEST")
	writeActorManifestBody(b, actors)
}

func writeActorManifestBody(b *strings.Builder, actors []ActorManifestItem) {
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

func writePlanningConstraintsBody(b *strings.Builder) {
	fmt.Fprintln(b, "- Revise only the current bounded packet.")
	fmt.Fprintln(b, "- StoryPacket closure rule: every choice target_node_id must point to a node in this same packet; future packet handoffs must be terminal nodes with revision_notes, not missing target ids.")
	fmt.Fprintln(b, "- Keep all branches outside Honcho until the Narrator rolls one out.")
	fmt.Fprintln(b, "- Preserve the scenario contract's must_honor items and avoid must_not_violate drift.")
	fmt.Fprintln(b, "- Make each major choice express the scenario choice_principles.")
	fmt.Fprintln(b, "- Let binding doctrine sharpen desire, opposition, or cost inside the player character's lived order.")
	fmt.Fprintln(b, "- Make strange material feel lived-in through internal cause, social practice, emotional credibility, visible cost, and consequence.")
	fmt.Fprintln(b, "- Actor agents are available only for ids listed in STORY ACTOR MANIFEST; otherwise keep actor_needs empty and write behavior as scene beats.")
	fmt.Fprintln(b, "- When registered story actors are present and character pressure matters, cast them with actor_needs on the playable node so the Director rehearsal can make the scene lively; otherwise put a concise no actor rehearsal reason in writer_notes.")
	fmt.Fprintln(b, "- Use actor cards and directional representations only for story actors explicitly present in the packet.")
	fmt.Fprintln(b, "- Treat lore references as support for the active scenario, not as automatic all-cast inventory.")
}

func renderedSection(id string, title string, required bool, priority int, minTokens int, write func(*strings.Builder)) contextSection {
	var b strings.Builder
	if write != nil {
		write(&b)
	}
	body := strings.TrimSpace(b.String())
	if body == "" {
		return contextSection{}
	}
	return contextSection{
		ID:        id,
		Title:     title,
		Text:      "## " + title + "\n" + body,
		Required:  required,
		Priority:  priority,
		MinTokens: minTokens,
	}
}

func nonEmptySections(sections []contextSection) []contextSection {
	out := make([]contextSection, 0, len(sections))
	for _, section := range sections {
		if strings.TrimSpace(section.Text) != "" {
			out = append(out, section)
		}
	}
	return out
}

func renderSectionTexts(sections []contextSection) string {
	var texts []string
	for _, section := range sections {
		if strings.TrimSpace(section.Text) != "" {
			texts = append(texts, strings.TrimSpace(section.Text))
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n\n"))
}

func renderBudgetedSections(role string, sections []contextSection, budget modelcap.Budget) (string, RenderBudgetReport) {
	full := renderSectionTexts(sections)
	fullTokens := modelcap.EstimateTokens(full)
	rendered := append([]contextSection(nil), sections...)
	decisions := sectionDecisions(sections, rendered)
	warnings := append([]string(nil), budget.Warnings...)
	if budget.InputBudgetTokens > 0 && fullTokens > budget.InputBudgetTokens {
		rendered, decisions = compactSections(sections, budget.InputBudgetTokens)
	}
	inline := renderSectionTexts(rendered)
	inlineTokens := modelcap.EstimateTokens(inline)
	if budget.InputBudgetTokens > 0 && inlineTokens > budget.InputBudgetTokens {
		warnings = appendStable(warnings, "context still exceeds input budget after section compaction")
	}
	budget.EstimatedPromptTokens = inlineTokens
	return inline, RenderBudgetReport{
		Role:                role,
		FullContextChars:    len([]rune(full)),
		InlineContextChars:  len([]rune(inline)),
		FullContextTokens:   fullTokens,
		InlineContextTokens: inlineTokens,
		Budget:              budget,
		Sections:            decisions,
		Warnings:            uniqueStrings(warnings),
	}
}

func compactSections(sections []contextSection, budgetTokens int) ([]contextSection, []SectionDecision) {
	rendered := append([]contextSection(nil), sections...)
	for modelcap.EstimateTokens(renderSectionTexts(rendered)) > budgetTokens {
		index, ok := nextCompactCandidate(rendered, false)
		if !ok {
			index, ok = nextCompactCandidate(rendered, true)
		}
		if !ok {
			break
		}
		section := rendered[index]
		sectionTokens := modelcap.EstimateTokens(section.Text)
		minTokens := section.MinTokens
		if minTokens <= 0 {
			minTokens = 200
		}
		if sectionTokens > minTokens {
			over := modelcap.EstimateTokens(renderSectionTexts(rendered)) - budgetTokens
			target := sectionTokens - over
			if target < minTokens {
				target = minTokens
			}
			rendered[index].Text = trimSectionText(section.Text, target)
			if modelcap.EstimateTokens(rendered[index].Text) < sectionTokens {
				continue
			}
		}
		if !section.Required {
			rendered[index].Text = ""
			continue
		}
		break
	}
	return rendered, sectionDecisions(sections, rendered)
}

func nextCompactCandidate(sections []contextSection, includeRequired bool) (int, bool) {
	best := -1
	for i, section := range sections {
		if strings.TrimSpace(section.Text) == "" {
			continue
		}
		if section.Required && !includeRequired {
			continue
		}
		if best < 0 || section.Priority < sections[best].Priority || (section.Priority == sections[best].Priority && modelcap.EstimateTokens(section.Text) > modelcap.EstimateTokens(sections[best].Text)) {
			best = i
		}
	}
	return best, best >= 0
}

func trimSectionText(text string, targetTokens int) string {
	if targetTokens <= 0 || modelcap.EstimateTokens(text) <= targetTokens {
		return text
	}
	notice := "\n\n[section trimmed for context budget]"
	maxRunes := targetTokens * 4
	runes := []rune(text)
	if maxRunes <= len([]rune(notice))+20 {
		maxRunes = len([]rune(notice)) + 20
	}
	if maxRunes >= len(runes) {
		return text
	}
	cut := string(runes[:maxRunes-len([]rune(notice))])
	if idx := strings.LastIndex(cut, "\n"); idx > 0 {
		cut = cut[:idx]
	}
	return strings.TrimSpace(cut) + notice
}

func sectionDecisions(original []contextSection, rendered []contextSection) []SectionDecision {
	out := make([]SectionDecision, 0, len(original))
	for i, section := range original {
		renderedText := ""
		if i < len(rendered) {
			renderedText = rendered[i].Text
		}
		status := "kept"
		if strings.TrimSpace(renderedText) == "" {
			status = "dropped"
		} else if renderedText != section.Text {
			status = "trimmed"
		}
		out = append(out, SectionDecision{
			ID:             section.ID,
			Title:          section.Title,
			Status:         status,
			Required:       section.Required,
			OriginalTokens: modelcap.EstimateTokens(section.Text),
			RenderedTokens: modelcap.EstimateTokens(renderedText),
		})
	}
	return out
}
