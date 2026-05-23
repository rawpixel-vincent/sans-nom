package story

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type Workspace struct {
	Root    string
	StoryID string
	Dir     string
}

type InitOptions struct {
	StoryID              string
	Root                 string
	GroundLorePackID     string
	GroundLoreCollection string
	GlobalLoreCollection string
	StoryCollection      string
	Replace              bool
}

func NewWorkspace(root string, storyID string) (Workspace, error) {
	if root == "" {
		root = DefaultRoot
	}
	storyID = SanitizeStoryID(storyID)
	if storyID == "" {
		return Workspace{}, errors.New("story_id is required")
	}
	return Workspace{
		Root:    root,
		StoryID: storyID,
		Dir:     filepath.Join(root, storyID),
	}, nil
}

func InitWorkspace(opts InitOptions, bundle SeedBundle) (Workspace, StoryState, error) {
	storyID := SanitizeStoryID(opts.StoryID)
	if storyID == "" {
		storyID = GenerateStoryID()
	}
	if bundle.Bible.StoryID == "" {
		bundle.Bible.StoryID = storyID
	}
	bundle.Bible.StoryID = storyID
	bundle.Packet.ContextSnapshot.WorkspaceID = storyID
	if bundle.Packet.ContextSnapshot.SessionID == "" {
		bundle.Packet.ContextSnapshot.SessionID = "main"
	}
	if err := ValidateBundle(bundle); err != nil {
		return Workspace{}, StoryState{}, err
	}
	ws, err := NewWorkspace(opts.Root, storyID)
	if err != nil {
		return Workspace{}, StoryState{}, err
	}
	if _, err := os.Stat(ws.Dir); err == nil && !opts.Replace {
		return Workspace{}, StoryState{}, fmt.Errorf("story %q already exists; use -replace-story to recreate it", storyID)
	}
	if opts.Replace {
		if err := os.RemoveAll(ws.Dir); err != nil {
			return Workspace{}, StoryState{}, err
		}
	}
	for _, dir := range []string{
		ws.Dir,
		ws.PacketsDir(),
		ws.ModelRunsDir(),
		ws.NarrationDir(),
		ws.ActorCardsDir(),
		ws.IndexDir(),
		ws.LogsDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Workspace{}, StoryState{}, err
		}
	}
	now := NowString()
	groundLoreCollection := firstNonEmpty(opts.GroundLoreCollection, opts.GlobalLoreCollection, DefaultGroundLoreCollection)
	state := StoryState{
		StoryID:              storyID,
		WorkspaceID:          storyID,
		SessionID:            bundle.Packet.ContextSnapshot.SessionID,
		StoryCollection:      firstNonEmpty(opts.StoryCollection, CollectionName(storyID)),
		GroundLorePackID:     firstNonEmpty(opts.GroundLorePackID, DefaultGroundLorePackID),
		GroundLoreCollection: groundLoreCollection,
		GlobalLoreCollection: strings.TrimSpace(opts.GlobalLoreCollection),
		BiblePath:            "bible.json",
		CurrentPacketID:      bundle.Packet.ID,
		CurrentPacketPath:    filepath.ToSlash(filepath.Join("packets", bundle.Packet.ID+".json")),
		CurrentNodeID:        bundle.Packet.EntryNodeID,
		Status:               StatusActive,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if state.CurrentNodeID == "" {
		if entry, err := bundle.Packet.EntryNode(); err == nil {
			state.CurrentNodeID = entry.ID
		}
	}
	if err := ws.SaveBible(bundle.Bible); err != nil {
		return Workspace{}, StoryState{}, err
	}
	if err := ws.SavePacket(bundle.Packet); err != nil {
		return Workspace{}, StoryState{}, err
	}
	if err := ws.SaveModelRun("seed-bundle", bundle); err != nil {
		return Workspace{}, StoryState{}, err
	}
	docs := BuildIndexDocuments(bundle.Bible, bundle.Packet, nil)
	if err := ws.WriteIndex(docs, state.StoryCollection); err != nil {
		return Workspace{}, StoryState{}, err
	}
	if err := ws.SaveState(state); err != nil {
		return Workspace{}, StoryState{}, err
	}
	return ws, state, nil
}

func LoadWorkspace(root string, storyID string) (Workspace, StoryState, error) {
	ws, err := NewWorkspace(root, storyID)
	if err != nil {
		return Workspace{}, StoryState{}, err
	}
	state, err := ws.LoadState()
	if err != nil {
		return Workspace{}, StoryState{}, err
	}
	if normalized, changed := state.WithDefaults(); changed {
		state = normalized
		if err := ws.SaveState(state); err != nil {
			return Workspace{}, StoryState{}, err
		}
	}
	return ws, state, nil
}

func (w Workspace) PacketsDir() string {
	return filepath.Join(w.Dir, "packets")
}

func (w Workspace) ModelRunsDir() string {
	return filepath.Join(w.Dir, "model-runs")
}

func (w Workspace) NarrationDir() string {
	return filepath.Join(w.Dir, "narration")
}

func (w Workspace) ActorCardsDir() string {
	return filepath.Join(w.Dir, "cards", "actors")
}

func (w Workspace) IndexDir() string {
	return filepath.Join(w.Dir, "index")
}

func (w Workspace) LogsDir() string {
	return filepath.Join(w.Dir, "logs")
}

func (w Workspace) SaveBible(bible StoryBible) error {
	return writeJSONAtomic(filepath.Join(w.Dir, "bible.json"), bible)
}

func (w Workspace) LoadBible() (StoryBible, error) {
	var bible StoryBible
	return bible, readJSON(filepath.Join(w.Dir, "bible.json"), &bible)
}

func (w Workspace) SavePacket(packet scene.StoryPacket) error {
	return writeJSONAtomic(filepath.Join(w.PacketsDir(), packet.ID+".json"), packet)
}

func (w Workspace) LoadPacket(state StoryState) (scene.StoryPacket, error) {
	path := state.CurrentPacketPath
	if path == "" && state.CurrentPacketID != "" {
		path = filepath.Join("packets", state.CurrentPacketID+".json")
	}
	var packet scene.StoryPacket
	err := readJSON(filepath.Join(w.Dir, filepath.FromSlash(path)), &packet)
	return packet, err
}

func (w Workspace) SaveState(state StoryState) error {
	state.UpdatedAt = NowString()
	return writeJSONAtomic(filepath.Join(w.Dir, "state.json"), state)
}

func (w Workspace) LoadState() (StoryState, error) {
	var state StoryState
	err := readJSON(filepath.Join(w.Dir, "state.json"), &state)
	return state, err
}

func (w Workspace) SaveModelRun(name string, value any) error {
	name = SanitizeStoryID(name)
	if name == "" {
		name = "run"
	}
	return writeJSONAtomic(filepath.Join(w.ModelRunsDir(), name+".json"), value)
}

func (w Workspace) AppendTurn(record TurnRecord) error {
	if record.CreatedAt == "" {
		record.CreatedAt = NowString()
	}
	path := filepath.Join(w.Dir, "turns.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	return enc.Encode(record)
}

func (w Workspace) LoadTurns() ([]TurnRecord, error) {
	path := filepath.Join(w.Dir, "turns.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []TurnRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record TurnRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, scanner.Err()
}

func (w Workspace) SavePendingTurn(record TurnRecord) (string, error) {
	if record.CreatedAt == "" {
		record.CreatedAt = NowString()
	}
	rel := "pending-turn.json"
	if err := writeJSONAtomic(filepath.Join(w.Dir, rel), record); err != nil {
		return "", err
	}
	if err := w.writeTurnNarration(filepath.ToSlash(filepath.Join("narration", "pending-turn.md")), record, true); err != nil {
		return "", err
	}
	return rel, nil
}

func (w Workspace) LoadPendingTurn(state StoryState) (TurnRecord, error) {
	rel := strings.TrimSpace(state.PendingTurnPath)
	if rel == "" {
		rel = "pending-turn.json"
	}
	var record TurnRecord
	err := readJSON(filepath.Join(w.Dir, filepath.FromSlash(rel)), &record)
	return record, err
}

func (w Workspace) RecordTurn(state StoryState, packet scene.StoryPacket, record TurnRecord) (StoryState, error) {
	if record.StoryID == "" {
		record.StoryID = state.StoryID
	}
	if record.Turn == 0 {
		record.Turn = state.LatestTurn + 1
	}
	pendingTurnPath := state.PendingTurnPath
	if err := w.AppendTurn(record); err != nil {
		return state, err
	}
	if err := w.SavePacket(packet); err != nil {
		return state, err
	}
	if _, err := w.SaveTurnNarration(record); err != nil {
		return state, err
	}
	state.LatestTurn = record.Turn
	state.CurrentPacketID = packet.ID
	state.CurrentPacketPath = filepath.ToSlash(filepath.Join("packets", packet.ID+".json"))
	if record.NextNodeID != "" {
		state.CurrentNodeID = record.NextNodeID
	} else {
		state.CurrentNodeID = record.NodeID
	}
	state.LastDecision = record.Decision
	state.LastNarratorProse = record.Prose
	state.PendingTurnPath = ""
	if state.Status == StatusAwaitingPlayerChoice || state.Status == StatusAwaitingContinuation {
		state.Status = StatusActive
	}
	if state.Status != StatusBlockedWriterRefresh &&
		((!PlayerDecisionEmpty(record.Decision) && PlayerDecisionEqual(record.Decision, state.PendingDecision)) ||
			(!PlayerDecisionEmpty(record.NextDecision) && PlayerDecisionEqual(record.NextDecision, state.PendingDecision))) {
		state.PendingDecision = scene.PlayerDecision{}
		state.PendingDecisionAt = ""
		state.PendingWriterRunID = ""
		state.PendingWriterRunPath = ""
	}
	if err := w.SaveState(state); err != nil {
		return state, err
	}
	if pendingTurnPath != "" {
		_ = os.Remove(filepath.Join(w.Dir, filepath.FromSlash(pendingTurnPath)))
		_ = os.Remove(filepath.Join(w.NarrationDir(), "pending-turn.md"))
	}
	return state, nil
}

func (w Workspace) SaveTurnNarration(record TurnRecord) (string, error) {
	if record.Turn <= 0 {
		return "", errors.New("turn narration requires a positive turn number")
	}
	rel := filepath.ToSlash(filepath.Join("narration", fmt.Sprintf("turn-%06d.md", record.Turn)))
	return rel, w.writeTurnNarration(rel, record, false)
}

func (w Workspace) LoadNarrationFiles() ([]NarrationFile, error) {
	entries, err := os.ReadDir(w.NarrationDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []NarrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(w.NarrationDir(), entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rel := filepath.ToSlash(filepath.Join("narration", entry.Name()))
		files = append(files, NarrationFile{
			Path:    rel,
			Name:    entry.Name(),
			Turn:    turnFromNarrationName(entry.Name()),
			Pending: entry.Name() == "pending-turn.md",
			Text:    string(data),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339Nano),
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Pending != files[j].Pending {
			return files[i].Pending
		}
		if files[i].Turn != files[j].Turn {
			return files[i].Turn < files[j].Turn
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func (w Workspace) writeTurnNarration(rel string, record TurnRecord, pending bool) error {
	if rel == "" {
		return errors.New("narration path is required")
	}
	var b strings.Builder
	title := "Turn"
	if pending {
		title = "Pending Turn"
	}
	if record.Turn > 0 {
		fmt.Fprintf(&b, "# %s %d\n\n", title, record.Turn)
	} else {
		fmt.Fprintf(&b, "# %s\n\n", title)
	}
	fmt.Fprintf(&b, "<!-- story_id: %s packet_id: %s node_id: %s -->\n\n", record.StoryID, record.PacketID, record.NodeID)
	if text := strings.TrimSpace(record.Prose); text != "" {
		b.WriteString(text)
		b.WriteString("\n")
	}
	if len(record.Choices) > 0 {
		b.WriteString("\n## Choices\n\n")
		for i, choice := range record.Choices {
			fmt.Fprintf(&b, "%d. **%s**: %s\n", i+1, choice.ID, strings.TrimSpace(choice.Label))
		}
	}
	if !PlayerDecisionEmpty(record.NextDecision) {
		b.WriteString("\n## Player Choice\n\n")
		if record.NextDecision.ChoiceID != "" {
			fmt.Fprintf(&b, "- Choice: %s\n", record.NextDecision.ChoiceID)
		}
		if record.NextDecision.FreeText != "" {
			fmt.Fprintf(&b, "- Free text: %s\n", record.NextDecision.FreeText)
		}
	}
	if record.AudioPath != "" {
		fmt.Fprintf(&b, "\nAudio: `%s`\n", filepath.ToSlash(record.AudioPath))
	}
	return writeTextAtomic(filepath.Join(w.Dir, filepath.FromSlash(rel)), b.String())
}

func turnFromNarrationName(name string) int {
	name = strings.TrimSuffix(name, ".md")
	name = strings.TrimPrefix(name, "turn-")
	turn, err := strconv.Atoi(name)
	if err != nil {
		return 0
	}
	return turn
}

func GenerateStoryID() string {
	return "story-" + time.Now().UTC().Format("20060102-150405")
}

func SanitizeStoryID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	return value
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	encodeErr := enc.Encode(value)
	closeErr := file.Close()
	if encodeErr != nil {
		_ = os.Remove(tmp)
		return encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, path)
}

func writeTextAtomic(path string, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(out)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
