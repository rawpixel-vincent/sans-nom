package contextbundle

import (
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func buildStoryProgression(bible *story.StoryBible, packet scene.StoryPacket, selectedNodeID string) StoryProgression {
	progression := StoryProgression{
		StoryArc: append([]string(nil), storyArcFromBible(bible)...),
	}
	var packetSpentMotifs []string
	if packet.Metadata != nil {
		progression.CurrentPhase = firstNonEmptyProgressionValue(packet.Metadata["story_phase"], packet.Metadata["arc_phase"], packet.Metadata["current_arc_phase"])
		progression.NextPhase = firstNonEmptyProgressionValue(packet.Metadata["next_story_phase"], packet.Metadata["next_arc_phase"])
		packetSpentMotifs = splitProgressionList(packet.Metadata["spent_motifs"])
		progression.SpentMotifs = appendUniqueStrings(progression.SpentMotifs, packetSpentMotifs...)
	}
	var openThreads []string
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
				openThreads = append(openThreads, thread)
			}
		}
	}
	progression.RequiredMovement = append(progression.RequiredMovement, selectedNodeCanonMovements(packet, selectedNodeID)...)
	progression.RequiredMovement = append(progression.RequiredMovement, openThreadMovements(openThreads, packetSpentMotifs)...)
	progression.RequiredMovement = appendUniqueStrings(progression.RequiredMovement, requiredMovements(progression)...)
	return progression
}

func selectedNodeCanonMovements(packet scene.StoryPacket, selectedNodeID string) []string {
	node, err := packet.Node(selectedNodeID)
	if err != nil {
		return nil
	}
	var out []string
	for _, intent := range node.CanonMemoryIntents() {
		if summary := strings.TrimSpace(intent.Summary); summary != "" {
			out = append(out, "Continue from selected node canon: "+summary)
		}
	}
	return out
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

func openThreadMovements(openThreads []string, packetSpentMotifs []string) []string {
	if len(openThreads) == 0 {
		return nil
	}
	if shouldNarrowOpenThreads(openThreads, packetSpentMotifs) {
		out := []string{
			"Narrowing phase: close, disprove, merge, or make costly at least one existing open thread before opening a new one.",
			"Do not introduce a new investigation shadow unless it directly resolves or transforms one of the listed open threads.",
		}
		for _, thread := range openThreads {
			out = append(out, "Open thread available for closure: "+thread)
		}
		return out
	}
	out := make([]string, 0, len(openThreads))
	for _, thread := range openThreads {
		out = append(out, "Keep open thread active: "+thread)
	}
	return out
}

func shouldNarrowOpenThreads(openThreads []string, packetSpentMotifs []string) bool {
	return len(openThreads) >= 3 && len(packetSpentMotifs) >= 3
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

func splitProgressionList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ';', '|', '\n':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			out = append(out, field)
		}
	}
	return out
}

func requiredMovements(progression StoryProgression) []string {
	out := []string{
		"Advance at least one macro state: location, custody/control, revealed information, opponent advantage, resource status, relationship state, or irreversible cost.",
		"Do not spend the next packet replaying the same situation without consequence; make something happen or move to a pressure where it matters.",
	}
	if progression.NextPhase != "" {
		out = append(out, "Use next phase guidance if it helps the scene move: "+progression.NextPhase)
	}
	if len(progression.SpentMotifs) > 0 {
		out = append(out, "Treat spent motifs as soft guidance: if one returns, change consequence, agency, relationship, location, or resource state.")
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
