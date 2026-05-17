package gamelog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	Time     string         `json:"time"`
	Level    string         `json:"level"`
	Category string         `json:"category"`
	Event    string         `json:"event"`
	StoryID  string         `json:"story_id,omitempty"`
	Turn     int            `json:"turn,omitempty"`
	PacketID string         `json:"packet_id,omitempty"`
	NodeID   string         `json:"node_id,omitempty"`
	RunID    string         `json:"run_id,omitempty"`
	Message  string         `json:"message,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

type Filter struct {
	Level    string
	Category string
	Event    string
	Tail     int
}

type Writer struct {
	Path    string
	StoryID string
}

func NewWriter(storyDir string, storyID string) *Writer {
	if strings.TrimSpace(storyDir) == "" {
		return nil
	}
	return &Writer{
		Path:    filepath.Join(storyDir, "logs", "events.jsonl"),
		StoryID: strings.TrimSpace(storyID),
	}
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
	if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
		return err
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
