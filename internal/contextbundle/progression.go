package contextbundle

import (
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func buildStoryProgression(bible *story.StoryBible, packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision, recentScenes []string) StoryProgression {
	progression := StoryProgression{
		StoryArc: append([]string(nil), storyArcFromBible(bible)...),
	}
	if packet.Metadata != nil {
		progression.CurrentPhase = firstNonEmptyProgressionValue(packet.Metadata["story_phase"], packet.Metadata["arc_phase"], packet.Metadata["current_arc_phase"])
		progression.NextPhase = firstNonEmptyProgressionValue(packet.Metadata["next_story_phase"], packet.Metadata["next_arc_phase"])
	}
	if bible != nil {
		if progression.CurrentPhase == "" {
			progression.CurrentPhase = strings.TrimSpace(bible.CurrentArcPhase)
		}
		progression.SpentMotifs = appendUniqueStrings(progression.SpentMotifs, bible.SpentMotifs...)
		if strings.TrimSpace(bible.NextRequiredRevelation) != "" {
			progression.RequiredMovement = append(progression.RequiredMovement, "Next required revelation: "+strings.TrimSpace(bible.NextRequiredRevelation))
		}
		if strings.TrimSpace(bible.NextRequiredOpposition) != "" {
			progression.RequiredMovement = append(progression.RequiredMovement, "Next required opposition: "+strings.TrimSpace(bible.NextRequiredOpposition))
		}
		for _, thread := range bible.OpenThreads {
			if thread = strings.TrimSpace(thread); thread != "" {
				progression.RequiredMovement = append(progression.RequiredMovement, "Keep open thread active: "+thread)
			}
		}
	}
	if progression.CurrentPhase == "" && len(progression.StoryArc) > 0 {
		progression.CurrentPhase = progression.StoryArc[0]
	}
	if progression.NextPhase == "" && len(progression.StoryArc) > 1 {
		progression.NextPhase = progression.StoryArc[1]
	}

	sourceText := progressionSignalText(packet, node, decision, recentScenes)
	progression.SpentMotifs = appendUniqueStrings(progression.SpentMotifs, detectSpentMotifs(sourceText)...)
	progression.RequiredMovement = appendUniqueStrings(progression.RequiredMovement, requiredMovements(progression)...)
	if containsAllMotifs(progression.SpentMotifs, "witness/testimony proof", "servant/household memory") {
		progression.Warnings = append(progression.Warnings, "Servant-memory proof appears spent; the next packet should make someone act on it or move to a different macro pressure.")
	}
	return progression
}

func appendUniqueStrings(values []string, more ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values)+len(more))
	for _, value := range append(values, more...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmptyProgressionValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func storyArcFromBible(bible *story.StoryBible) []string {
	if bible == nil {
		return nil
	}
	var out []string
	for _, step := range bible.StoryArc {
		if step = strings.TrimSpace(step); step != "" {
			out = append(out, step)
		}
	}
	return out
}

func progressionSignalText(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision, recentScenes []string) string {
	var parts []string
	parts = append(parts, recentScenes...)
	parts = append(parts, packet.ContextSnapshot.LatestNarratorProse)
	parts = append(parts, packet.ContextSnapshot.LastUserDecision)
	parts = append(parts, node.Title)
	for _, beat := range node.Beats {
		parts = append(parts, beat.Text)
	}
	for _, choice := range node.Choices {
		parts = append(parts, choice.ID, choice.Label)
	}
	parts = append(parts, decision.ChoiceID, decision.FreeText)
	return strings.ToLower(strings.Join(parts, "\n"))
}

func detectSpentMotifs(text string) []string {
	checks := []struct {
		name     string
		keywords []string
	}{
		{"witness/testimony proof", []string{"witness", "testimony", "testify", "ledger", "record", "memory", "remember", "mask"}},
		{"servant/household memory", []string{"servant", "household", "steward", "notary", "scribe", "service"}},
		{"rite/body cost", []string{"rite", "body", "blood", "wound", "scar", "hand", "skin", "cost"}},
		{"dynasty/inheritance pressure", []string{"dynasty", "heir", "succession", "inheritance", "house", "family", "lineage"}},
		{"weapon/resource claim", []string{"weapon", "blade", "key-blade", "armory", "locker", "knife", "resource"}},
		{"rival/family authority", []string{"rival", "claimant", "father", "mother", "patriarch", "matriarch", "authority"}},
	}
	var out []string
	for _, check := range checks {
		for _, keyword := range check.keywords {
			if strings.Contains(text, keyword) {
				out = append(out, check.name)
				break
			}
		}
	}
	return out
}

func requiredMovements(progression StoryProgression) []string {
	out := []string{
		"Advance at least one macro state: location, custody/control, revealed truth, opponent advantage, resource/weapon status, witness record, or irreversible cost.",
		"Do not spend the next packet re-proving a mechanism already accepted by witnessed canon; make the consequence happen.",
	}
	if progression.NextPhase != "" {
		out = append(out, "Aim the next packet toward story_arc next phase: "+progression.NextPhase)
	}
	if containsAllMotifs(progression.SpentMotifs, "witness/testimony proof", "servant/household memory") {
		out = append(out, "Do not center another servant ledger, mask, copy, or testimony-validity debate unless the player explicitly asks to delay.")
		out = append(out, "Shift pressure toward weapon/resource claim, public contest, rival claimant, family authority, lineage revelation, or a new location where the proof has consequences.")
	}
	return out
}

func containsAllMotifs(values []string, needles ...string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, needle := range needles {
		if !seen[needle] {
			return false
		}
	}
	return true
}
