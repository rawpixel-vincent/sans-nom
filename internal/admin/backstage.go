package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
)

type backstageMessage struct {
	ID             string   `json:"id"`
	Time           string   `json:"time"`
	StoryID        string   `json:"story_id,omitempty"`
	Turn           int      `json:"turn,omitempty"`
	RunID          string   `json:"run_id,omitempty"`
	Role           string   `json:"role"`
	Speaker        string   `json:"speaker"`
	Kind           string   `json:"kind"`
	Status         string   `json:"status"`
	Text           string   `json:"text"`
	Preview        string   `json:"preview"`
	SourceEventID  string   `json:"source_event_id,omitempty"`
	DetailPath     string   `json:"detail_path,omitempty"`
	ArtifactPaths  []string `json:"artifact_paths,omitempty"`
	Visibility     string   `json:"visibility"`
	PacketID       string   `json:"packet_id,omitempty"`
	NodeID         string   `json:"node_id,omitempty"`
	SourceEvent    string   `json:"source_event,omitempty"`
	SourceCategory string   `json:"source_category,omitempty"`
}

type backstageBuilder struct {
	root   string
	writer *gamelog.Writer
}

type backstageCaller struct {
	Role    string
	Speaker string
}

func buildBackstageMessages(root string, writer *gamelog.Writer, events []gamelog.Event) []backstageMessage {
	builder := backstageBuilder{root: filepath.Clean(root), writer: writer}
	messages := []backstageMessage{}
	activeCaller := backstageCaller{}
	for _, event := range events {
		messages = append(messages, builder.messagesForEvent(event, activeCaller)...)
		if caller, ok := eventCaller(event); ok {
			activeCaller = caller
		}
	}
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Time == messages[j].Time {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].Time < messages[j].Time
	})
	return messages
}

func (b backstageBuilder) messagesForEvent(event gamelog.Event, activeCaller backstageCaller) []backstageMessage {
	detail := b.detail(event)
	switch event.Event {
	case "seed.workshop.request":
		return []backstageMessage{b.message(event, "creative-writer", "System", "mention", "pending", "@CreativeWriter "+stringField(detail, "prompt"))}
	case "seed.workshop.response":
		return []backstageMessage{b.message(event, "creative-writer", "Creative Writer", "response", "completed", stringField(detail, "output"))}
	case "seed.workshop.failed":
		return []backstageMessage{b.message(event, "creative-writer", "Creative Writer", "error", "failed", errorText(event, detail))}
	case "seed.writer.request":
		return []backstageMessage{b.message(event, "creative-writer", "System", "mention", "pending", "@CreativeWriter "+stringField(detail, "prompt"))}
	case "seed.writer.response":
		return []backstageMessage{b.message(event, "creative-writer", "Creative Writer", "response", "completed", stringField(detail, "response"))}
	case "seed.writer.invalid", "seed.bundle.invalid":
		return []backstageMessage{b.message(event, "creative-writer", "System", "error", "failed", errorText(event, detail))}
	case "seed.verifier.request":
		return []backstageMessage{b.message(event, "nerd-verifier", "System", "mention", "pending", "@NerdVerifier "+stringField(detail, "prompt"))}
	case "seed.verifier.response":
		return []backstageMessage{b.message(event, "nerd-verifier", "Nerd Verifier", "response", "completed", stringField(detail, "response"))}
	case "writer.refresh.started":
		return []backstageMessage{b.writerArtifactMessage(event, "System", "mention", "pending", "@Writer "+firstNonEmpty(readArtifactText(b.root, event, "artifact_dir", "input.txt"), event.Message))}
	case "writer.refresh.completed":
		return []backstageMessage{b.writerArtifactMessage(event, "Writer", "response", "completed", firstNonEmpty(readArtifactText(b.root, event, "artifact_dir", "accepted-packet.json"), readArtifactText(b.root, event, "artifact_dir", "raw-output.json"), event.Message))}
	case "writer.refresh.failed":
		return []backstageMessage{b.writerArtifactMessage(event, "Writer", "error", "failed", firstNonEmpty(readArtifactText(b.root, event, "artifact_dir", "validation-error.txt"), readArtifactText(b.root, event, "artifact_dir", "raw-output.json"), errorText(event, detail)))}
	case "narrator.rollout.request":
		return []backstageMessage{b.message(event, "narrator", "System", "mention", "pending", "@Narrator "+firstNonEmpty(stringField(detail, "prompt"), stringField(detail, "context_text"), event.Message))}
	case "narrator.rollout.completed":
		return []backstageMessage{b.message(event, "narrator", "Narrator", "response", "completed", nestedString(detail, "draft", "prose"))}
	case "narrator.rollout.failed":
		return []backstageMessage{b.message(event, "narrator", "Narrator", "error", "failed", errorText(event, detail))}
	case "actor.invoke.request":
		caller := toolCaller(event, detail, activeCaller, backstageCaller{Role: "narrator", Speaker: "Narrator"})
		return []backstageMessage{b.message(event, caller.Role, caller.Speaker, "mention", "pending", "@"+firstNonEmpty(stringFromFields(event, "actor_id"), "Actor")+" "+firstNonEmpty(stringField(detail, "prompt"), event.Message))}
	case "actor.invoke.completed":
		return []backstageMessage{b.message(event, "actor", firstNonEmpty(stringFromFields(event, "actor_id"), "Actor"), "response", "completed", firstNonEmpty(marshalPretty(detail["output"]), event.Message))}
	case "actor.invoke.failed":
		return []backstageMessage{b.message(event, "actor", firstNonEmpty(stringFromFields(event, "actor_id"), "Actor"), "error", "failed", errorText(event, detail))}
	case "scenarist.call.started":
		caller := toolCaller(event, detail, activeCaller, defaultToolCaller(event))
		return []backstageMessage{b.message(event, caller.Role, caller.Speaker, "tool_request", "pending", "@Scenarist "+scenaristRequestText(detail))}
	case "scenarist.call.completed":
		return b.scenaristResponses(event, detail)
	case "scenarist.call.failed", "scenarist.call.rejected":
		return []backstageMessage{b.message(event, "scenarist", "Scenarist", "error", "failed", errorText(event, detail))}
	case "lore.expert.request":
		caller := toolCaller(event, detail, activeCaller, defaultToolCaller(event))
		return []backstageMessage{b.message(event, caller.Role, caller.Speaker, "tool_request", "pending", "@LoreExpert "+firstNonEmpty(nestedString(detail, "request", "question"), stringField(detail, "prompt"), event.Message))}
	case "lore.expert.response":
		return []backstageMessage{b.message(event, "lore-expert", "Lore Expert", "tool_response", "completed", firstNonEmpty(stringField(detail, "answer"), b.readPathText(nestedString(detail, "metadata", "answer_path")), event.Message))}
	case "lore.expert.cache_hit":
		return []backstageMessage{b.message(event, "lore-expert", "Lore Expert", "tool_response", "cached", firstNonEmpty(b.readPathText(stringField(detail, "answer_path")), event.Message))}
	case "lore.expert.failed":
		return []backstageMessage{b.message(event, "lore-expert", "Lore Expert", "error", "failed", errorText(event, detail))}
	case "tts.mimo.request":
		return []backstageMessage{b.message(event, "voice", "System", "mention", "pending", "@Voice "+event.Message)}
	case "tts.mimo.chunk.completed":
		return []backstageMessage{b.message(event, "voice", "Voice", "status", "completed", firstNonEmpty(marshalPretty(detail), event.Message))}
	case "tts.mimo.completed":
		return []backstageMessage{b.message(event, "voice", "Voice", "response", "completed", firstNonEmpty(nestedString(detail, "metadata", "output_path"), event.Message))}
	case "tts.mimo.failed":
		return []backstageMessage{b.message(event, "voice", "Voice", "error", "failed", errorText(event, detail))}
	default:
		if isBackstageCategory(event.Category) {
			status := "completed"
			kind := "status"
			if event.Level == gamelog.LevelError || event.Level == gamelog.LevelWarn {
				status = "failed"
				kind = "error"
			}
			return []backstageMessage{b.message(event, roleForCategory(event.Category), speakerForCategory(event.Category), kind, status, firstNonEmpty(event.Message, marshalPretty(detail), event.Event))}
		}
		return nil
	}
}

func (b backstageBuilder) message(event gamelog.Event, role string, speaker string, kind string, status string, text string) backstageMessage {
	text = strings.TrimSpace(text)
	if text == "" {
		text = firstNonEmpty(event.Message, event.Event)
	}
	artifacts := artifactPaths(b.root, event)
	return backstageMessage{
		ID:             backstageID(event, role, kind, text),
		Time:           event.Time,
		StoryID:        event.StoryID,
		Turn:           event.Turn,
		RunID:          event.RunID,
		Role:           role,
		Speaker:        speaker,
		Kind:           kind,
		Status:         status,
		Text:           text,
		Preview:        previewText(text, 240),
		SourceEventID:  event.EventID,
		DetailPath:     event.DetailPath,
		ArtifactPaths:  artifacts,
		Visibility:     "admin_only",
		PacketID:       event.PacketID,
		NodeID:         event.NodeID,
		SourceEvent:    event.Event,
		SourceCategory: event.Category,
	}
}

func (b backstageBuilder) writerArtifactMessage(event gamelog.Event, speaker string, kind string, status string, text string) backstageMessage {
	msg := b.message(event, "writer", speaker, kind, status, text)
	msg.ArtifactPaths = uniqueStrings(append(msg.ArtifactPaths, writerArtifactPaths(b.root, event)...))
	return msg
}

func (b backstageBuilder) scenaristResponses(event gamelog.Event, detail map[string]any) []backstageMessage {
	materials := nestedSlice(detail, "manifest", "materials")
	if len(materials) == 0 {
		return []backstageMessage{b.message(event, "scenarist", "Scenarist", "tool_response", "completed", event.Message)}
	}
	out := make([]backstageMessage, 0, len(materials))
	for _, raw := range materials {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := stringField(item, "path")
		text := firstNonEmpty(b.readPathText(path), stringField(item, "topic"), event.Message)
		msg := b.message(event, "scenarist", "Scenarist", "tool_response", "completed", text)
		msg.ID = backstageID(event, "scenarist", stringField(item, "topic_id"), text)
		msg.ArtifactPaths = uniqueStrings(append(msg.ArtifactPaths, path))
		out = append(out, msg)
	}
	return out
}

func (b backstageBuilder) detail(event gamelog.Event) map[string]any {
	if event.DetailPath == "" || b.writer == nil {
		return nil
	}
	detail, err := b.writer.ReadDetail(event.DetailPath)
	if err != nil {
		return nil
	}
	if mapped, ok := detail.(map[string]any); ok {
		return mapped
	}
	return map[string]any{"detail": detail}
}

func isBackstageCategory(category string) bool {
	switch category {
	case "model", "writer", "narrator", "actor", "scenarist", "lore":
		return true
	default:
		return false
	}
}

func roleForCategory(category string) string {
	switch category {
	case "model":
		return "system"
	case "lore":
		return "lore-expert"
	default:
		return category
	}
}

func speakerForCategory(category string) string {
	switch category {
	case "model":
		return "System"
	case "lore":
		return "Lore Expert"
	default:
		return titleWords(strings.ReplaceAll(category, "-", " "))
	}
}

func eventCaller(event gamelog.Event) (backstageCaller, bool) {
	switch event.Event {
	case "seed.workshop.request", "seed.writer.request":
		return backstageCaller{Role: "creative-writer", Speaker: "Creative Writer"}, true
	case "writer.refresh.started":
		return backstageCaller{Role: "writer", Speaker: "Writer"}, true
	case "narrator.rollout.request":
		return backstageCaller{Role: "narrator", Speaker: "Narrator"}, true
	default:
		return backstageCaller{}, false
	}
}

func toolCaller(event gamelog.Event, detail map[string]any, active backstageCaller, fallback backstageCaller) backstageCaller {
	role := firstNonEmpty(stringFromFields(event, "caller_role"), stringField(detail, "caller_role"), active.Role, fallback.Role)
	speaker := firstNonEmpty(stringFromFields(event, "caller_speaker"), stringField(detail, "caller_speaker"), active.Speaker, fallback.Speaker, speakerForRole(role))
	return backstageCaller{Role: role, Speaker: speaker}
}

func defaultToolCaller(event gamelog.Event) backstageCaller {
	if event.StoryID == "" {
		return backstageCaller{Role: "creative-writer", Speaker: "Creative Writer"}
	}
	return backstageCaller{Role: "writer", Speaker: "Writer"}
}

func speakerForRole(role string) string {
	switch role {
	case "creative-writer":
		return "Creative Writer"
	case "writer":
		return "Writer"
	case "narrator":
		return "Narrator"
	case "lore-expert":
		return "Lore Expert"
	case "scenarist":
		return "Scenarist"
	default:
		return titleWords(strings.ReplaceAll(role, "-", " "))
	}
}

func backstageID(event gamelog.Event, role string, kind string, text string) string {
	seed := event.EventID + "|" + role + "|" + kind + "|" + previewText(text, 120)
	if strings.TrimSpace(seed) == "|||" {
		seed = event.Time + "|" + event.Event
	}
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:12])
}

func scenaristRequestText(detail map[string]any) string {
	requests := sliceField(detail, "requests")
	if len(requests) == 0 {
		return stringField(detail, "arguments")
	}
	var lines []string
	for _, raw := range requests {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		lines = append(lines, "- "+firstNonEmpty(stringField(item, "topic"), stringField(item, "topic_id"))+": "+stringField(item, "request"))
	}
	return strings.Join(lines, "\n")
}

func errorText(event gamelog.Event, detail map[string]any) string {
	return firstNonEmpty(stringField(detail, "error"), stringField(detail, "validation_error"), event.Message, event.Event)
}

func artifactPaths(root string, event gamelog.Event) []string {
	var out []string
	for _, key := range []string{"artifact_dir", "run_dir", "output_path", "prompt_path", "answer_path", "call_dir"} {
		if value := stringFromFields(event, key); value != "" {
			out = append(out, normalizeArtifactPath(root, value))
		}
	}
	return uniqueStrings(out)
}

func writerArtifactPaths(root string, event gamelog.Event) []string {
	dir := stringFromFields(event, "artifact_dir")
	if dir == "" {
		return nil
	}
	var out []string
	for _, name := range []string{"input.txt", "raw-output.json", "accepted-packet.json", "validation-error.txt", "metadata.json"} {
		path, err := safeArtifactPath(root, filepath.Join(filepath.FromSlash(dir), name))
		if err == nil {
			if _, err := os.Stat(path); err == nil {
				out = append(out, normalizeArtifactPath(root, path))
			}
		}
	}
	return out
}

func readArtifactText(root string, event gamelog.Event, dirField string, name string) string {
	dir := stringFromFields(event, dirField)
	if dir == "" {
		return ""
	}
	return readRootedPathText(root, filepath.Join(filepath.FromSlash(dir), name))
}

func (b backstageBuilder) readPathText(path string) string {
	return readRootedPathText(b.root, path)
}

func readRootedPathText(root string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	safe, err := safeArtifactPath(root, path)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(safe)
	if err != nil {
		return ""
	}
	return string(data)
}

func normalizeArtifactPath(root string, path string) string {
	safe, err := safeArtifactPath(root, path)
	if err != nil {
		return ""
	}
	absRoot, rootErr := filepath.Abs(root)
	absPath, pathErr := filepath.Abs(safe)
	if rootErr == nil && pathErr == nil {
		if rel, err := filepath.Rel(absRoot, absPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(safe)
}

func safeArtifactPath(root string, path string) (string, error) {
	root = filepath.Clean(root)
	path = filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if root == "." || path == "." || path == "" {
		return "", fmt.Errorf("path is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	if filepath.IsAbs(path) {
		candidates = append(candidates, path)
	} else {
		candidates = append(candidates, filepath.Join(root, path), path)
	}
	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, absPath)
		if err != nil {
			continue
		}
		if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
			continue
		}
		return absPath, nil
	}
	return "", fmt.Errorf("path escapes backstage root")
}

func stringFromFields(event gamelog.Event, key string) string {
	if event.Fields == nil {
		return ""
	}
	value, _ := event.Fields[key].(string)
	return strings.TrimSpace(value)
}

func stringField(value any, key string) string {
	mapped, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	str, _ := mapped[key].(string)
	return strings.TrimSpace(str)
}

func nestedString(value any, keys ...string) string {
	current := value
	for _, key := range keys {
		mapped, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = mapped[key]
	}
	str, _ := current.(string)
	return strings.TrimSpace(str)
}

func sliceField(value any, key string) []any {
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	slice, _ := mapped[key].([]any)
	return slice
}

func nestedSlice(value any, keys ...string) []any {
	current := value
	for _, key := range keys {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapped[key]
	}
	slice, _ := current.([]any)
	return slice
}

func marshalPretty(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func previewText(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
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
	return out
}

func titleWords(value string) string {
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
