package writer

import (
	"fmt"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func ValidateProgression(input Input, packet scene.StoryPacket) error {
	if err := validatePlayerFocusedChoices(packet); err != nil {
		return err
	}
	if err := validateActorRehearsal(input, packet); err != nil {
		return err
	}
	if !hasRepeatedSituationRisk(input.SpentMotifs) {
		return nil
	}
	entry, err := packet.EntryNode()
	if err != nil {
		return err
	}
	if len(entry.Choices) < 3 {
		return nil
	}
	repeatOnly := 0
	for _, choice := range entry.Choices {
		text := strings.ToLower(choiceBranchText(packet, choice))
		if repetitionManagementText(text) && !repeatedSituationShiftText(text) {
			repeatOnly++
		}
	}
	if repeatOnly == len(entry.Choices) {
		return fmt.Errorf("writer packet repeats a spent situation: all %d entry choices are only variants of carrying, copying, hiding, preserving, or restating the same material", len(entry.Choices))
	}
	if !packetIntroducesConsequenceShift(packet) {
		return fmt.Errorf("writer packet repeats a spent situation: add consequence, new pressure, changed relationship, location/status shift, resource change, or opponent action before revisiting the same mechanism")
	}
	return nil
}

func validateActorRehearsal(input Input, packet scene.StoryPacket) error {
	registered := stringSetFrom(input.RegisteredActorIDs)
	if len(registered) == 0 {
		return nil
	}
	entry, err := packet.EntryNode()
	if err != nil {
		return err
	}
	if len(entry.ActorNeeds) > 0 || hasNoActorRehearsalReason(entry.WriterNotes) {
		return nil
	}
	for _, actorID := range packet.ContextSnapshot.PresentActors {
		if _, ok := registered[actorKey(actorID)]; ok {
			return fmt.Errorf("writer packet needs Director rehearsal: registered actor %q is present on playable entry node %q, so add actor_needs or a concise no actor rehearsal reason in writer_notes", actorID, entry.ID)
		}
	}
	return nil
}

func hasNoActorRehearsalReason(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(normalized, "no actor rehearsal")
}

func validatePlayerFocusedChoices(packet scene.StoryPacket) error {
	for _, node := range packet.Nodes {
		if len(node.Choices) < 2 {
			continue
		}
		for _, choice := range node.Choices {
			if !playerIntentIsMeaningful(choice.PlayerIntent) {
				return fmt.Errorf("writer packet choice %q in node %q needs player-centered player_intent that changes pressure, loyalty, cost, knowledge, or agency", choice.ID, node.ID)
			}
		}
	}
	return nil
}

func playerIntentIsMeaningful(intent string) bool {
	text := strings.ToLower(strings.TrimSpace(intent))
	if len([]rune(text)) < 20 {
		return false
	}
	generic := strings.Trim(text, ". ")
	for _, value := range []string{"continue", "go", "wait", "observe", "look", "listen", "move", "talk", "choose", "do this"} {
		if generic == value {
			return false
		}
	}
	for _, keyword := range []string{
		"agency", "betray", "body", "challenge", "claim", "control", "cost", "desire", "duty", "knowledge", "loyal", "pressure", "protect", "refuse", "reveal", "risk", "sacrifice", "save", "status", "trust", "truth",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func stringSetFrom(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = actorKey(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func actorKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func hasRepeatedSituationRisk(motifs []string) bool {
	if len(motifs) >= 2 {
		return true
	}
	for _, motif := range motifs {
		text := strings.ToLower(strings.TrimSpace(motif))
		if strings.Contains(text, "repeat") || strings.Contains(text, "same situation") || strings.Contains(text, "spent") {
			return true
		}
	}
	return false
}

func packetIntroducesConsequenceShift(packet scene.StoryPacket) bool {
	return repeatedSituationShiftText(strings.ToLower(packetText(packet)))
}

func packetText(packet scene.StoryPacket) string {
	var parts []string
	parts = append(parts, packet.ID, packet.EntryNodeID)
	parts = append(parts, packet.ContextSnapshot.Location, packet.ContextSnapshot.Time, packet.ContextSnapshot.LatestNarratorProse, packet.ContextSnapshot.LastUserDecision)
	for _, value := range packet.ContextSnapshot.RelevantLoreSnippets {
		parts = append(parts, value)
	}
	for key, value := range packet.Metadata {
		parts = append(parts, key, value)
	}
	for _, node := range packet.Nodes {
		parts = append(parts, nodeText(node))
	}
	return strings.Join(parts, "\n")
}

func choiceBranchText(packet scene.StoryPacket, choice scene.Choice) string {
	parts := []string{choice.ID, choice.Label, choice.PlayerIntent, choice.TargetNodeID}
	if choice.TargetNodeID != "" {
		if node, err := packet.Node(choice.TargetNodeID); err == nil {
			parts = append(parts, nodeText(node))
		}
	}
	return strings.Join(parts, "\n")
}

func nodeText(node scene.SceneNode) string {
	var parts []string
	parts = append(parts, node.ID, node.Title, node.WriterNotes)
	for _, beat := range node.Beats {
		parts = append(parts, beat.ID, beat.Text)
	}
	for _, choice := range node.Choices {
		parts = append(parts, choice.ID, choice.Label, choice.PlayerIntent, choice.TargetNodeID)
	}
	for _, intent := range node.MemoryIntents {
		parts = append(parts, intent.ID, intent.Summary)
	}
	parts = append(parts, node.RevisionNotes...)
	parts = append(parts, node.InvalidationNotes...)
	return strings.Join(parts, "\n")
}

func repetitionManagementText(text string) bool {
	for _, keyword := range []string{
		"archive", "carry", "conceal", "copy", "hide", "inventory", "log", "mark", "preserve", "record", "repeat", "restore", "same", "store",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func consequenceShiftText(text string) bool {
	for _, keyword := range []string{
		"ambush", "arrest", "authority", "body", "capture", "challenge", "contest", "cost", "custody", "danger", "decision", "discovery", "escape", "exile", "expose", "gate", "injury", "irreversible", "location", "opponent", "public", "relationship", "resource", "rival", "scar", "status", "tool", "wound",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func repeatedSituationShiftText(text string) bool {
	for _, keyword := range []string{
		"ambush", "arrest", "authority", "body", "capture", "challenge", "contest", "custody", "danger", "decision", "discovery", "escape", "exile", "expose", "gate", "injury", "irreversible", "location", "opponent", "public", "relationship", "resource", "rival", "scar", "tool", "wound",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
