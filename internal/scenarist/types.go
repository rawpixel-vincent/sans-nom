package scenarist

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxToolRequests = 24

type ToolRequest struct {
	TopicID        string   `json:"topic_id"`
	Topic          string   `json:"topic"`
	Request        string   `json:"request"`
	HardBoundaries []string `json:"hard_boundaries"`
	RelevantPaths  []string `json:"relevant_paths"`
}

type Material struct {
	TopicID string `json:"topic_id"`
	Topic   string `json:"topic"`
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
}

type Manifest struct {
	Root      string     `json:"root"`
	Materials []Material `json:"materials"`
}

func WriteManifest(path string, manifest Manifest) error {
	return writeJSONAtomic(path, manifest)
}

func (r ToolRequest) Validate() error {
	required := map[string]string{
		"topic_id": r.TopicID,
		"topic":    r.Topic,
		"request":  r.Request,
	}
	for label, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
	}
	if SanitizeID(r.TopicID) == "" {
		return errors.New("topic_id must contain at least one letter or number")
	}
	if len(nonEmpty(r.HardBoundaries)) == 0 {
		return errors.New("scenarist request needs hard boundaries")
	}
	return nil
}

func SanitizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func MaterialPath(root string, topicID string) string {
	return filepath.Join(root, SanitizeID(topicID)+".md")
}

func nonEmpty(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
