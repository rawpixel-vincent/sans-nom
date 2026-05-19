package agentfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Scope describes the filesystem view exposed to a model role for one run.
type Scope struct {
	Root         string
	Role         string
	StoryID      string
	SeedRunID    string
	VisiblePaths []string
}

func New(root string, role string) Scope {
	return Scope{
		Root: filepath.Clean(root),
		Role: strings.TrimSpace(role),
	}
}

func (s Scope) WithStoryID(storyID string) Scope {
	s.StoryID = strings.TrimSpace(storyID)
	return s
}

func (s Scope) WithSeedRunID(seedRunID string) Scope {
	s.SeedRunID = strings.TrimSpace(seedRunID)
	return s
}

func (s Scope) WithVisiblePaths(paths ...string) Scope {
	for _, path := range paths {
		rel, err := s.Normalize(path)
		if err == nil && rel != "" {
			s.VisiblePaths = appendUnique(s.VisiblePaths, rel)
		}
	}
	return s
}

func (s Scope) Ensure() error {
	if strings.TrimSpace(s.Root) == "" {
		return errors.New("agent workspace root is required")
	}
	return os.MkdirAll(s.Root, 0o755)
}

func (s Scope) Abs(rel string) (string, error) {
	normalized, err := s.Normalize(rel)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Root, filepath.FromSlash(normalized)), nil
}

func (s Scope) Normalize(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return "", err
	}
	var abs string
	if filepath.IsAbs(path) {
		abs, err = filepath.Abs(path)
		if err != nil {
			return "", err
		}
	} else {
		clean := filepath.Clean(filepath.FromSlash(path))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("path escapes agent workspace: %s", path)
		}
		if ForbiddenRelativePath(clean) {
			return "", fmt.Errorf("path is outside the curated agent workspace: %s", path)
		}
		abs = filepath.Join(root, clean)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes agent workspace: %s", path)
	}
	if ForbiddenRelativePath(rel) {
		return "", fmt.Errorf("path is outside the curated agent workspace: %s", path)
	}
	return filepath.ToSlash(rel), nil
}

func (s Scope) StageText(rel string, text string) (string, error) {
	path, err := s.Abs(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return s.Normalize(path)
}

func (s Scope) StageJSON(rel string, value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return s.StageText(rel, string(data)+"\n")
}

func (s Scope) StageFile(source string, rel string) (string, error) {
	target, err := s.Abs(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return s.Normalize(target)
}

func ForbiddenRelativePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "." {
		return false
	}
	forbidden := []string{
		".git",
		"cmd",
		"internal",
		"testdata",
		"lore",
		"data/stories",
		"data/seed-runs",
	}
	for _, prefix := range forbidden {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
