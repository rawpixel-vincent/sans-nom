package gamelog

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/telemetry"
)

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

var categories = map[string]struct{}{
	"system":    {},
	"story":     {},
	"writer":    {},
	"narrator":  {},
	"actor":     {},
	"rehearsal": {},
	"scenarist": {},
	"model":     {},
	"lore":      {},
	"bridge":    {},
	"honcho":    {},
	"qdrant":    {},
	"memory":    {},
	"control":   {},
	"ui":        {},
}

type Event struct {
	EventID    string         `json:"event_id"`
	Time       string         `json:"time"`
	Level      string         `json:"level"`
	Category   string         `json:"category"`
	Event      string         `json:"event"`
	StoryID    string         `json:"story_id,omitempty"`
	Turn       int            `json:"turn,omitempty"`
	PacketID   string         `json:"packet_id,omitempty"`
	NodeID     string         `json:"node_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	Message    string         `json:"message,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
	Detail     any            `json:"-"`
	DetailPath string         `json:"detail_path,omitempty"`
}

type Filter struct {
	Level    string
	Category string
	Event    string
	Tail     int
}

type Writer struct {
	RootDir string
	Path    string
	StoryID string
}

func NewWriter(storyDir string, storyID string) *Writer {
	if strings.TrimSpace(storyDir) == "" {
		return nil
	}
	return &Writer{
		RootDir: filepath.Clean(storyDir),
		Path:    filepath.Join(storyDir, "logs", "events.jsonl"),
		StoryID: strings.TrimSpace(storyID),
	}
}

func NewGlobalWriter(storyRoot string) *Writer {
	if strings.TrimSpace(storyRoot) == "" {
		return nil
	}
	root := filepath.Clean(filepath.Dir(filepath.Clean(storyRoot)))
	return &Writer{
		RootDir: root,
		Path:    filepath.Join(root, "logs", "events.jsonl"),
	}
}

func GlobalEventsPath(storyRoot string) string {
	writer := NewGlobalWriter(storyRoot)
	if writer == nil {
		return ""
	}
	return writer.Path
}

func Categories() []string {
	out := make([]string, 0, len(categories))
	for category := range categories {
		out = append(out, category)
	}
	sort.Strings(out)
	return out
}

func ValidateCategory(category string) error {
	if _, ok := categories[category]; !ok {
		return errors.New("unknown log category: " + category)
	}
	return nil
}

func (w *Writer) Append(event Event) error {
	if w == nil || strings.TrimSpace(w.Path) == "" {
		return nil
	}
	if event.Time == "" {
		event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.EventID == "" {
		event.EventID = newEventID(event.Time)
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}
	if event.Category == "" {
		event.Category = "system"
	}
	if err := ValidateCategory(event.Category); err != nil {
		return err
	}
	if event.StoryID == "" {
		event.StoryID = w.StoryID
	}
	event.Fields = RedactMap(event.Fields)
	if event.Detail != nil && event.DetailPath == "" {
		event.DetailPath = filepath.ToSlash(filepath.Join("logs", "details", event.EventID+".json"))
	}
	if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
		return err
	}
	if event.Detail != nil {
		if err := w.writeDetail(event.DetailPath, event.Detail); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(w.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	return enc.Encode(event)
}

func (w *Writer) ReadDetail(detailPath string) (any, error) {
	path, err := w.safeDetailPath(detailPath)
	if err != nil {
		return nil, err
	}
	var out any
	err = readJSON(path, &out)
	return out, err
}

func (w *Writer) Delete(eventID string) (Event, bool, error) {
	deleted, err := w.DeleteMany([]string{eventID})
	if err != nil {
		return Event{}, false, err
	}
	if len(deleted) == 0 {
		return Event{}, false, nil
	}
	return deleted[0], true, nil
}

func (w *Writer) DeleteMany(eventIDs []string) ([]Event, error) {
	if w == nil || strings.TrimSpace(w.Path) == "" {
		return nil, nil
	}
	targets := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		eventID = strings.TrimSpace(eventID)
		if eventID == "" {
			continue
		}
		targets[eventID] = struct{}{}
	}
	if len(targets) == 0 {
		return nil, errors.New("event_id is required")
	}
	file, err := os.Open(w.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var (
		events  []Event
		deleted []Event
	)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		if _, ok := targets[event.EventID]; ok {
			deleted = append(deleted, event)
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(deleted) == 0 {
		return nil, nil
	}
	if err := rewriteEvents(w.Path, events); err != nil {
		return nil, err
	}
	for _, event := range deleted {
		if event.DetailPath != "" {
			if path, err := w.safeDetailPath(event.DetailPath); err == nil {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return nil, err
				}
			}
		}
	}
	return deleted, nil
}

func (w *Writer) writeDetail(detailPath string, detail any) error {
	path, err := w.safeDetailPath(detailPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(redactValue(detail), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (w *Writer) safeDetailPath(detailPath string) (string, error) {
	if w == nil || strings.TrimSpace(w.RootDir) == "" {
		return "", errors.New("log writer root is required")
	}
	if strings.TrimSpace(detailPath) == "" {
		return "", errors.New("detail path is required")
	}
	root, err := filepath.Abs(w.RootDir)
	if err != nil {
		return "", err
	}
	path := filepath.Clean(filepath.FromSlash(detailPath))
	if filepath.IsAbs(path) {
		return "", errors.New("absolute detail paths are not allowed")
	}
	full := filepath.Join(root, path)
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("detail path escapes log root: %s", detailPath)
	}
	return full, nil
}

func (w *Writer) Emit(_ context.Context, event telemetry.Event) {
	if w == nil {
		return
	}
	category := "system"
	if strings.HasPrefix(event.Name, "lore.") {
		category = "lore"
	}
	_ = w.Append(Event{
		Level:    LevelInfo,
		Category: category,
		Event:    event.Name,
		Message:  event.Name,
		Fields:   event.Fields,
	})
}

func Read(path string, filter Filter) ([]Event, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		if !matches(event, filter) {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if filter.Tail > 0 && len(events) > filter.Tail {
		events = events[len(events)-filter.Tail:]
	}
	return events, nil
}

func matches(event Event, filter Filter) bool {
	if filter.Level != "" && event.Level != filter.Level {
		return false
	}
	if filter.Category != "" && event.Category != filter.Category {
		return false
	}
	if filter.Event != "" && event.Event != filter.Event {
		return false
	}
	return true
}

func RedactMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if secretKey(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = redactValue(value)
	}
	return out
}

func readJSON(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(out)
}

func rewriteEvents(path string, events []Event) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return RedactMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactValue(item)
		}
		return out
	case string:
		if secretString(typed) {
			return "[redacted]"
		}
		return typed
	default:
		return value
	}
}

func secretKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{"token", "secret", "api_key", "apikey", "authorization", "password", "oauth", "refresh"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func secretString(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "bearer ") || strings.HasPrefix(lower, "sk-") || strings.Contains(lower, "api_key=")
}

func newEventID(timestamp string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return strings.NewReplacer(":", "", "-", "", ".", "").Replace(timestamp) + "-" + hex.EncodeToString(b[:])
	}
	return strings.NewReplacer(":", "", "-", "", ".", "").Replace(timestamp)
}
