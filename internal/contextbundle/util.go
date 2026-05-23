package contextbundle

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func queryText(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision) string {
	var b strings.Builder
	writeLine(&b, "location", packet.ContextSnapshot.Location)
	writeLine(&b, "time", packet.ContextSnapshot.Time)
	writeLine(&b, "latest_narrator_prose", packet.ContextSnapshot.LatestNarratorProse)
	writeLine(&b, "last_user_decision", packet.ContextSnapshot.LastUserDecision)
	if decision.ChoiceID != "" {
		writeLine(&b, "choice_id", decision.ChoiceID)
	}
	if decision.FreeText != "" {
		writeLine(&b, "free_text", decision.FreeText)
	}
	if len(packet.ContextSnapshot.PresentActors) > 0 {
		writeLine(&b, "present_actors", strings.Join(sortedCopy(packet.ContextSnapshot.PresentActors), ", "))
	}
	if node.ID != "" {
		writeLine(&b, "node", node.ID)
		writeLine(&b, "title", node.Title)
		for _, beat := range node.Beats {
			writeLine(&b, "beat", beat.Text)
		}
		for _, choice := range node.Choices {
			writeLine(&b, "choice", choice.ID+" "+choice.Label+" "+choice.PlayerIntent)
		}
	}
	return strings.TrimSpace(b.String())
}

func writeLine(b *strings.Builder, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s: %s\n", key, value)
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func trimToCount[T any](values []T, count int) []T {
	if count <= 0 || len(values) <= count {
		return values
	}
	return append([]T(nil), values[:count]...)
}

func trimStrings(values []string, maxChars int, warnings *[]string, label string) []string {
	out := make([]string, 0, len(values))
	for i, value := range values {
		out = append(out, truncate(value, maxChars, warnings, fmt.Sprintf("%s %d", label, i+1)))
	}
	return out
}

func truncate(value string, max int, warnings *[]string, label string) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	if warnings != nil {
		*warnings = appendStable(*warnings, label+" truncated")
	}
	if max < 4 {
		return value[:max]
	}
	return strings.TrimSpace(value[:max-3]) + "..."
}

func appendStable(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func uniqueStrings(values []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func nonEmptyStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func sortLore(hits []LoreHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Score > hits[j].Score
	})
}

func decisionText(decision scene.PlayerDecision) string {
	if decision.FreeText != "" {
		return "free_text: " + decision.FreeText
	}
	if decision.ChoiceID != "" {
		return "choice_id: " + decision.ChoiceID
	}
	return "observe"
}
