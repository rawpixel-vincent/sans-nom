package lorepack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultPackID                      = "metabarons"
	packsDir                           = "lore/packs"
	GameplayModeFaithfulCanonPrototype = "faithful_canon_prototype"
)

type Pack struct {
	ID                    string            `json:"id"`
	Label                 string            `json:"label"`
	Description           string            `json:"description"`
	Collection            string            `json:"collection"`
	EmbeddingModel        string            `json:"embedding_model"`
	ArtifactRoots         map[string]string `json:"artifact_roots"`
	Builder               string            `json:"builder"`
	SmokeQueries          []SmokeQuery      `json:"smoke_queries"`
	ForbiddenLiteralTerms []string          `json:"forbidden_literal_terms"`
	Status                string            `json:"status"`
	GameplayMode          string            `json:"gameplay_mode,omitempty"`
	GameSynopsis          string            `json:"game_synopsis,omitempty"`
	ProtagonistRole       string            `json:"protagonist_role,omitempty"`
	OpeningSituation      string            `json:"opening_situation,omitempty"`
	InitialPressure       string            `json:"initial_pressure,omitempty"`
	StoryLines            []string          `json:"story_lines,omitempty"`
	MustHonor             []string          `json:"must_honor,omitempty"`
	MustNotViolate        []string          `json:"must_not_violate,omitempty"`
	ChoicePrinciples      []string          `json:"choice_principles,omitempty"`
	Path                  string            `json:"path,omitempty"`
}

type SmokeQuery struct {
	Query       string     `json:"query"`
	ExpectedAny [][]string `json:"expected_any"`
}

type ScenarioContract struct {
	GameplayMode     string   `json:"gameplay_mode"`
	GameSynopsis     string   `json:"game_synopsis"`
	ProtagonistRole  string   `json:"protagonist_role"`
	OpeningSituation string   `json:"opening_situation"`
	InitialPressure  string   `json:"initial_pressure"`
	StoryLines       []string `json:"story_lines"`
	MustHonor        []string `json:"must_honor"`
	MustNotViolate   []string `json:"must_not_violate"`
	ChoicePrinciples []string `json:"choice_principles"`
}

func DefaultPackID() string {
	return defaultPackID
}

func ListPacks(root string) ([]Pack, error) {
	root = normalizeRoot(root)
	entries, err := os.ReadDir(filepath.Join(root, packsDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var packs []Pack
	seen := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, packsDir, entry.Name(), "lore-pack.json")
		pack, err := readPack(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if previous := seen[pack.ID]; previous != "" {
			return nil, fmt.Errorf("duplicate lore pack id %q in %s and %s", pack.ID, previous, path)
		}
		seen[pack.ID] = path
		packs = append(packs, pack)
	}
	sort.SliceStable(packs, func(i, j int) bool {
		if packRank(packs[i]) != packRank(packs[j]) {
			return packRank(packs[i]) < packRank(packs[j])
		}
		return packs[i].Label < packs[j].Label
	})
	return packs, nil
}

func GetPack(root string, id string) (Pack, error) {
	root = normalizeRoot(root)
	id = strings.TrimSpace(id)
	if id == "" {
		id = defaultPackID
	}
	pack, err := readPack(filepath.Join(root, packsDir, id, "lore-pack.json"))
	if err == nil {
		return pack, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Pack{}, err
	}
	packs, err := ListPacks(root)
	if err != nil {
		return Pack{}, err
	}
	for _, pack := range packs {
		if pack.ID == id {
			return pack, nil
		}
	}
	return Pack{}, fmt.Errorf("lore pack %q not found under %s", id, filepath.Join(root, packsDir))
}

func readPack(path string) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, err
	}
	var pack Pack
	if err := json.Unmarshal(data, &pack); err != nil {
		return Pack{}, fmt.Errorf("%s: %w", path, err)
	}
	pack.Path = filepath.Clean(path)
	if err := pack.Validate(); err != nil {
		return Pack{}, fmt.Errorf("%s: %w", path, err)
	}
	return pack, nil
}

func (p Pack) Validate() error {
	required := map[string]string{
		"id":         p.ID,
		"label":      p.Label,
		"collection": p.Collection,
		"builder":    p.Builder,
		"status":     p.Status,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if strings.ContainsAny(p.ID, `/\`) {
		return fmt.Errorf("id %q must not contain path separators", p.ID)
	}
	if len(p.ArtifactRoots) == 0 {
		return errors.New("artifact_roots is required")
	}
	if strings.TrimSpace(p.Status) == "active" {
		if err := p.validateActiveScenarioContract(); err != nil {
			return err
		}
	}
	return nil
}

func (p Pack) ScenarioContract() ScenarioContract {
	return ScenarioContract{
		GameplayMode:     p.GameplayMode,
		GameSynopsis:     p.GameSynopsis,
		ProtagonistRole:  p.ProtagonistRole,
		OpeningSituation: p.OpeningSituation,
		InitialPressure:  p.InitialPressure,
		StoryLines:       append([]string(nil), p.StoryLines...),
		MustHonor:        append([]string(nil), p.MustHonor...),
		MustNotViolate:   append([]string(nil), p.MustNotViolate...),
		ChoicePrinciples: append([]string(nil), p.ChoicePrinciples...),
	}
}

func (p Pack) validateActiveScenarioContract() error {
	required := map[string]string{
		"gameplay_mode":     p.GameplayMode,
		"game_synopsis":     p.GameSynopsis,
		"protagonist_role":  p.ProtagonistRole,
		"opening_situation": p.OpeningSituation,
		"initial_pressure":  p.InitialPressure,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required for active lore packs", name)
		}
	}
	if p.GameplayMode != GameplayModeFaithfulCanonPrototype {
		return fmt.Errorf("unsupported gameplay_mode %q", p.GameplayMode)
	}
	requiredLists := map[string][]string{
		"story_lines":       p.StoryLines,
		"must_honor":        p.MustHonor,
		"must_not_violate":  p.MustNotViolate,
		"choice_principles": p.ChoicePrinciples,
	}
	for name, values := range requiredLists {
		if !hasNonEmpty(values) {
			return fmt.Errorf("%s is required for active lore packs", name)
		}
	}
	return nil
}

func hasNonEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func packRank(pack Pack) int {
	if pack.ID == defaultPackID {
		return 0
	}
	if pack.Status == "active" {
		return 1
	}
	return 2
}

func normalizeRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return root
}
