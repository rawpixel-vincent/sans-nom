package story

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		ws.IndexDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Workspace{}, StoryState{}, err
		}
	}
	now := NowString()
	state := StoryState{
		StoryID:              storyID,
		WorkspaceID:          storyID,
		SessionID:            bundle.Packet.ContextSnapshot.SessionID,
		StoryCollection:      firstNonEmpty(opts.StoryCollection, CollectionName(storyID)),
		GlobalLoreCollection: firstNonEmpty(opts.GlobalLoreCollection, DefaultGlobalLoreCollection),
		BiblePath:            "bible.json",
		CurrentPacketID:      bundle.Packet.ID,
		CurrentPacketPath:    filepath.ToSlash(filepath.Join("packets", bundle.Packet.ID+".json")),
		CurrentNodeID:        bundle.Packet.EntryNodeID,
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
	return ws, state, nil
}

func (w Workspace) PacketsDir() string {
	return filepath.Join(w.Dir, "packets")
}

func (w Workspace) ModelRunsDir() string {
	return filepath.Join(w.Dir, "model-runs")
}

func (w Workspace) IndexDir() string {
	return filepath.Join(w.Dir, "index")
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

func (w Workspace) RecordTurn(state StoryState, packet scene.StoryPacket, record TurnRecord) (StoryState, error) {
	if record.StoryID == "" {
		record.StoryID = state.StoryID
	}
	if record.Turn == 0 {
		record.Turn = state.LatestTurn + 1
	}
	if err := w.AppendTurn(record); err != nil {
		return state, err
	}
	if err := w.SavePacket(packet); err != nil {
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
	return state, w.SaveState(state)
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

func textSHA256(text string) string {
	return hashText(text)
}
