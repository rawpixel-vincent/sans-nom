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
	defaultMasterLoreFile              = "LORE.md"
	GameplayModeCanonAdaptation        = "canon_adaptation"
	GameplayModeCanonInternalOriginal  = "canon_internal_original"
	GameplayModeHomageOriginal         = "homage_original"
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
	MustNotViolate        []string          `json:"must_not_violate,omitempty"`
	CanonAnchor           CanonAnchor       `json:"canon_anchor,omitempty"`
	Path                  string            `json:"path,omitempty"`
}

type SmokeQuery struct {
	Query       string     `json:"query"`
	ExpectedAny [][]string `json:"expected_any"`
}

type ScenarioContract struct {
	GameplayMode     string      `json:"gameplay_mode"`
	GameSynopsis     string      `json:"game_synopsis"`
	ProtagonistRole  string      `json:"protagonist_role"`
	OpeningSituation string      `json:"opening_situation"`
	InitialPressure  string      `json:"initial_pressure"`
	StoryLines       []string    `json:"story_lines"`
	MustNotViolate   []string    `json:"must_not_violate"`
	CanonAnchor      CanonAnchor `json:"canon_anchor,omitempty"`
}

type CanonAnchor struct {
	SourceVolumes             []int    `json:"source_volumes,omitempty"`
	SourceEvents              []string `json:"source_events,omitempty"`
	SourceCharacters          []string `json:"source_characters,omitempty"`
	MustIncludeCanonFacts     []string `json:"must_include_canon_facts,omitempty"`
	AllowedOriginalInsertions []string `json:"allowed_original_insertions,omitempty"`
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
		MustNotViolate:   append([]string(nil), p.MustNotViolate...),
		CanonAnchor:      p.CanonAnchor.Clone(),
	}
}

func (p Pack) ScenarioContractText() string {
	if strings.TrimSpace(p.GameplayMode) == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Lorepack Scenario Notes: %s (%s)\n", firstNonEmpty(p.Label, p.ID), firstNonEmpty(p.ID, "unknown"))
	fmt.Fprintf(&b, "- gameplay_mode: %s\n", p.GameplayMode)
	fmt.Fprintf(&b, "- game_synopsis: %s\n", strings.TrimSpace(p.GameSynopsis))
	fmt.Fprintf(&b, "- protagonist_role: %s\n", strings.TrimSpace(p.ProtagonistRole))
	fmt.Fprintf(&b, "- opening_situation: %s\n", strings.TrimSpace(p.OpeningSituation))
	fmt.Fprintf(&b, "- initial_pressure: %s\n", strings.TrimSpace(p.InitialPressure))
	writeScenarioList(&b, "story_lines", p.StoryLines)
	writeScenarioList(&b, "avoid_drift_notes", p.MustNotViolate)
	writeCanonAnchor(&b, p.CanonAnchor)
	return strings.TrimSpace(b.String())
}

func (p Pack) ScenarioOrientationText() string {
	if strings.TrimSpace(p.GameplayMode) == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Lorepack Scenario Orientation: %s (%s)\n", firstNonEmpty(p.Label, p.ID), firstNonEmpty(p.ID, "unknown"))
	fmt.Fprintf(&b, "- gameplay_mode: %s\n", p.GameplayMode)
	fmt.Fprintln(&b, "- contract_path: lorepack/lore-pack.json when local read tools are enabled")
	fmt.Fprintf(&b, "- master_lore_path: lorepack/%s; its content is injected into model system prompts when available\n", filepath.ToSlash(p.MasterLoreRelPath()))
	fmt.Fprintln(&b, "- use: treat authored scenario notes as fidelity/background support for play, not a fixed one-scene template")
	fmt.Fprintln(&b, "- fidelity: preserve its premise logic, protagonist pressure, source boundaries, and useful choice guidance")
	return strings.TrimSpace(b.String())
}

func (p Pack) MasterLoreRelPath() string {
	if p.ArtifactRoots != nil {
		if value := strings.TrimSpace(p.ArtifactRoots["summary"]); value != "" {
			return filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
		}
	}
	return defaultMasterLoreFile
}

func (p Pack) MasterLorePath() string {
	if strings.TrimSpace(p.Path) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p.Path), filepath.FromSlash(p.MasterLoreRelPath()))
}

func (p Pack) MasterLoreText() (string, error) {
	path := p.MasterLorePath()
	if strings.TrimSpace(path) == "" {
		return "", errors.New("lore pack path is required to read master lore")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return text, nil
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
	if !supportedGameplayMode(p.GameplayMode) {
		return fmt.Errorf("unsupported gameplay_mode %q", p.GameplayMode)
	}
	if p.GameplayMode == GameplayModeCanonAdaptation && p.CanonAnchor.Empty() {
		return errors.New("canon_anchor is required for canon_adaptation lore packs")
	}
	if err := p.validateMasterLore(); err != nil {
		return err
	}
	requiredLists := map[string][]string{
		"story_lines":      p.StoryLines,
		"must_not_violate": p.MustNotViolate,
	}
	for name, values := range requiredLists {
		if !hasNonEmpty(values) {
			return fmt.Errorf("%s is required for active lore packs", name)
		}
	}
	return nil
}

func (p Pack) validateMasterLore() error {
	rel := p.MasterLoreRelPath()
	if filepath.IsAbs(rel) {
		return fmt.Errorf("summary artifact path %q must be relative", rel)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return fmt.Errorf("summary artifact path %q must stay inside the lore pack", rel)
	}
	path := p.MasterLorePath()
	if strings.TrimSpace(path) == "" {
		return errors.New("LORE.md is required for active lore packs")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s is required for active lore packs: %w", filepath.ToSlash(rel), err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return fmt.Errorf("%s is required for active lore packs and must not be empty", filepath.ToSlash(rel))
	}
	return nil
}

func supportedGameplayMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case GameplayModeCanonAdaptation, GameplayModeCanonInternalOriginal, GameplayModeHomageOriginal, GameplayModeFaithfulCanonPrototype:
		return true
	default:
		return false
	}
}

func (a CanonAnchor) Empty() bool {
	return len(a.SourceVolumes) == 0 &&
		!hasNonEmpty(a.SourceEvents) &&
		!hasNonEmpty(a.SourceCharacters) &&
		!hasNonEmpty(a.MustIncludeCanonFacts) &&
		!hasNonEmpty(a.AllowedOriginalInsertions)
}

func (a CanonAnchor) Clone() CanonAnchor {
	return CanonAnchor{
		SourceVolumes:             append([]int(nil), a.SourceVolumes...),
		SourceEvents:              append([]string(nil), a.SourceEvents...),
		SourceCharacters:          append([]string(nil), a.SourceCharacters...),
		MustIncludeCanonFacts:     append([]string(nil), a.MustIncludeCanonFacts...),
		AllowedOriginalInsertions: append([]string(nil), a.AllowedOriginalInsertions...),
	}
}

func hasNonEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func writeCanonAnchor(b *strings.Builder, anchor CanonAnchor) {
	if anchor.Empty() {
		return
	}
	fmt.Fprintln(b, "- canon_anchor:")
	if len(anchor.SourceVolumes) > 0 {
		var values []string
		for _, volume := range anchor.SourceVolumes {
			values = append(values, fmt.Sprintf("%d", volume))
		}
		fmt.Fprintf(b, "  - source_volumes: %s\n", strings.Join(values, ", "))
	}
	writeScenarioNestedList(b, "source_events", anchor.SourceEvents)
	writeScenarioNestedList(b, "source_characters", anchor.SourceCharacters)
	writeScenarioNestedList(b, "must_include_canon_facts", anchor.MustIncludeCanonFacts)
	writeScenarioNestedList(b, "allowed_original_insertions", anchor.AllowedOriginalInsertions)
}

func writeScenarioNestedList(b *strings.Builder, label string, values []string) {
	if !hasNonEmpty(values) {
		return
	}
	fmt.Fprintf(b, "  - %s:\n", label)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			fmt.Fprintf(b, "    - %s\n", value)
		}
	}
}

func writeScenarioList(b *strings.Builder, label string, values []string) {
	if !hasNonEmpty(values) {
		return
	}
	fmt.Fprintf(b, "- %s:\n", label)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		fmt.Fprintf(b, "  - %s\n", value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
