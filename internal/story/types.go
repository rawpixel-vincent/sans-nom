package story

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

const (
	DefaultRoot                 = "data/stories"
	DefaultGroundLorePackID     = "metabarons"
	DefaultGroundLoreCollection = "sans_nom_lore_metabarons"
	DefaultGlobalLoreCollection = "sans_nom_lore"

	StatusActive               = "active"
	StatusAwaitingPlayerChoice = "awaiting_player_choice"
	StatusAwaitingContinuation = "awaiting_writer_continuation"
	StatusBlockedWriterRefresh = "blocked_writer_refresh"
)

var RequiredResumeMetadataKeys = []string{
	"seed_version",
	"generated_at",
	"resume_point",
	"current_checkpoint",
	"continuation_hooks",
	"canon_validated",
	"open_threads",
}

type SeedBundle struct {
	Bible  StoryBible        `json:"bible"`
	Packet scene.StoryPacket `json:"packet"`
}

type StoryBible struct {
	StoryID                    string            `json:"story_id"`
	Title                      string            `json:"title"`
	Premise                    string            `json:"premise"`
	PlayerCharacter            PlayerCharacter   `json:"player_character"`
	PlayerWorldConnection      string            `json:"player_world_connection"`
	WorldBasics                []string          `json:"world_basics"`
	StoryContext               []string          `json:"story_context,omitempty"`
	LorePrinciples             []OriginalLaw     `json:"lore_principles,omitempty"`
	OriginalLaws               []OriginalLaw     `json:"original_laws,omitempty"`
	Factions                   []Faction         `json:"factions"`
	CoreRelationships          []string          `json:"core_relationships"`
	ToneGuidance               []string          `json:"tone_guidance,omitempty"`
	ToneDoctrine               []string          `json:"tone_doctrine"`
	SourceLoreInfluenceRefs    []string          `json:"source_lore_influence_refs,omitempty"`
	ForbiddenLiteralCanonTerms []string          `json:"forbidden_literal_canon_terms,omitempty"`
	FirstTurnExposition        []string          `json:"first_turn_exposition"`
	StoryArc                   []string          `json:"story_arc,omitempty"`
	CurrentArcPhase            string            `json:"current_arc_phase,omitempty"`
	NextRequiredRevelation     string            `json:"next_required_revelation,omitempty"`
	NextRequiredOpposition     string            `json:"next_required_opposition,omitempty"`
	SpentMotifs                []string          `json:"spent_motifs,omitempty"`
	OpenThreads                []string          `json:"open_threads,omitempty"`
	ResumeSummary              string            `json:"resume_summary"`
	Metadata                   map[string]string `json:"metadata"`
}

type PlayerCharacter struct {
	PeerID      string `json:"peer_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Desire      string `json:"desire"`
	Wound       string `json:"wound"`
	Boundary    string `json:"boundary"`
}

type OriginalLaw struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	SourcePattern  string `json:"source_pattern"`
	Enforcement    string `json:"enforcement"`
	WhyProfound    string `json:"why_profound"`
	NotMerelyCruel string `json:"not_merely_cruel"`
}

func (b StoryBible) BindingLorePrinciples() []OriginalLaw {
	if len(b.LorePrinciples) > 0 {
		return b.LorePrinciples
	}
	return b.OriginalLaws
}

func (b StoryBible) StoryContextGuidance() []string {
	out := append([]string(nil), b.StoryContext...)
	for _, note := range b.BindingLorePrinciples() {
		var parts []string
		if strings.TrimSpace(note.Name) != "" {
			parts = append(parts, strings.TrimSpace(note.Name))
		}
		if strings.TrimSpace(note.Description) != "" {
			parts = append(parts, strings.TrimSpace(note.Description))
		}
		if strings.TrimSpace(note.SourcePattern) != "" {
			parts = append(parts, "source pattern: "+strings.TrimSpace(note.SourcePattern))
		}
		if strings.TrimSpace(note.Enforcement) != "" {
			parts = append(parts, "consequence note: "+strings.TrimSpace(note.Enforcement))
		}
		if strings.TrimSpace(note.WhyProfound) != "" {
			parts = append(parts, "weight note: "+strings.TrimSpace(note.WhyProfound))
		}
		if strings.TrimSpace(note.NotMerelyCruel) != "" {
			parts = append(parts, "care note: "+strings.TrimSpace(note.NotMerelyCruel))
		}
		if len(parts) > 0 {
			out = append(out, strings.Join(parts, " | "))
		}
	}
	return out
}

func (b StoryBible) ToneGuidanceLines() []string {
	if len(b.ToneGuidance) > 0 {
		return b.ToneGuidance
	}
	return b.ToneDoctrine
}

type Faction struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type StoryState struct {
	StoryID              string               `json:"story_id"`
	WorkspaceID          string               `json:"workspace_id"`
	SessionID            string               `json:"session_id"`
	StoryCollection      string               `json:"story_collection"`
	GroundLorePackID     string               `json:"ground_lore_pack_id,omitempty"`
	GroundLoreCollection string               `json:"ground_lore_collection,omitempty"`
	GlobalLoreCollection string               `json:"global_lore_collection,omitempty"`
	BiblePath            string               `json:"bible_path"`
	CurrentPacketID      string               `json:"current_packet_id"`
	CurrentPacketPath    string               `json:"current_packet_path"`
	CurrentNodeID        string               `json:"current_node_id"`
	Status               string               `json:"status,omitempty"`
	LatestTurn           int                  `json:"latest_turn"`
	LastDecision         scene.PlayerDecision `json:"last_decision"`
	LastNarratorProse    string               `json:"last_narrator_prose"`
	PendingDecision      scene.PlayerDecision `json:"pending_decision,omitempty"`
	PendingDecisionAt    string               `json:"pending_decision_at,omitempty"`
	PendingTurnPath      string               `json:"pending_turn_path,omitempty"`
	PendingCardUpdates   []scene.CardUpdate   `json:"pending_card_updates,omitempty"`
	PendingCardUpdatesAt string               `json:"pending_card_updates_at,omitempty"`
	PendingWriterRunID   string               `json:"pending_writer_run_id,omitempty"`
	PendingWriterRunPath string               `json:"pending_writer_run_path,omitempty"`
	BlockedReason        string               `json:"blocked_reason,omitempty"`
	BlockedAt            string               `json:"blocked_at,omitempty"`
	BlockedWriterRunID   string               `json:"blocked_writer_run_id,omitempty"`
	BlockedWriterRunPath string               `json:"blocked_writer_run_path,omitempty"`
	CreatedAt            string               `json:"created_at"`
	UpdatedAt            string               `json:"updated_at"`
}

func (s StoryState) SourceLorePackID() string {
	if strings.TrimSpace(s.GroundLorePackID) != "" {
		return s.GroundLorePackID
	}
	return DefaultGroundLorePackID
}

func (s StoryState) SourceLoreCollection() string {
	if strings.TrimSpace(s.GroundLoreCollection) != "" {
		return s.GroundLoreCollection
	}
	if collection := strings.TrimSpace(s.GlobalLoreCollection); collection != "" && collection != DefaultGlobalLoreCollection {
		return collection
	}
	return DefaultGroundLoreCollection
}

func (s StoryState) WithDefaults() (StoryState, bool) {
	out := s
	changed := false
	if strings.TrimSpace(out.GroundLorePackID) == "" {
		out.GroundLorePackID = DefaultGroundLorePackID
		changed = true
	}
	if strings.TrimSpace(out.GroundLoreCollection) == "" {
		out.GroundLoreCollection = out.SourceLoreCollection()
		changed = true
	}
	if strings.TrimSpace(out.Status) == "" {
		out.Status = StatusActive
		changed = true
	}
	return out, changed
}

func (s StoryState) HasPendingDecision() bool {
	return !PlayerDecisionEmpty(s.PendingDecision)
}

func PlayerDecisionEmpty(decision scene.PlayerDecision) bool {
	return strings.TrimSpace(decision.ChoiceID) == "" && strings.TrimSpace(decision.FreeText) == ""
}

func PlayerDecisionEqual(a scene.PlayerDecision, b scene.PlayerDecision) bool {
	return a.ChoiceID == b.ChoiceID && a.FreeText == b.FreeText
}

type TurnRecord struct {
	Turn         int                  `json:"turn"`
	StoryID      string               `json:"story_id"`
	PacketID     string               `json:"packet_id"`
	NodeID       string               `json:"node_id"`
	Decision     scene.PlayerDecision `json:"decision"`
	NextDecision scene.PlayerDecision `json:"next_decision,omitempty"`
	Prose        string               `json:"prose"`
	Choices      []scene.Choice       `json:"choices"`
	Commit       scene.Commit         `json:"commit"`
	CardUpdates  []scene.CardUpdate   `json:"card_updates,omitempty"`
	Committed    bool                 `json:"committed"`
	NextNodeID   string               `json:"next_node_id,omitempty"`
	AudioPath    string               `json:"audio_path,omitempty"`
	CreatedAt    string               `json:"created_at"`
}

type NarrationFile struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Turn    int    `json:"turn,omitempty"`
	Pending bool   `json:"pending,omitempty"`
	Text    string `json:"text"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func ValidateBundle(bundle SeedBundle) error {
	if err := bundle.Bible.Validate(); err != nil {
		return fmt.Errorf("story bible: %w", err)
	}
	if err := ValidateSeedPacket(bundle.Packet); err != nil {
		return err
	}
	return nil
}

func (b StoryBible) Validate() error {
	required := map[string]string{
		"title":                    b.Title,
		"premise":                  b.Premise,
		"player character name":    b.PlayerCharacter.Name,
		"player character peer id": b.PlayerCharacter.PeerID,
		"player description":       b.PlayerCharacter.Description,
		"player desire":            b.PlayerCharacter.Desire,
		"player wound":             b.PlayerCharacter.Wound,
		"player boundary":          b.PlayerCharacter.Boundary,
		"player world connection":  b.PlayerWorldConnection,
		"resume summary":           b.ResumeSummary,
	}
	for label, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
	}
	if len(b.WorldBasics) < 3 {
		return fmt.Errorf("story bible needs at least 3 world basics")
	}
	if len(b.Metadata) == 0 {
		return errors.New("story bible metadata is required for resume handoff")
	}
	for _, key := range RequiredResumeMetadataKeys {
		if strings.TrimSpace(b.Metadata[key]) == "" {
			return fmt.Errorf("story bible metadata field %q is required for resume handoff", key)
		}
	}
	return nil
}

func ValidateSeedPacket(packet scene.StoryPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if len(packet.Nodes) == 0 {
		return errors.New("seed packet needs at least one scene node")
	}
	entry, err := packet.EntryNode()
	if err != nil {
		return err
	}
	if len(entry.Beats) == 0 {
		return fmt.Errorf("seed entry node %q needs at least one beat", entry.ID)
	}
	if err := validateSeedAgency(packet); err != nil {
		return err
	}
	return nil
}

func validateSeedAgency(packet scene.StoryPacket) error {
	hasChoicePoint := false
	for _, node := range packet.Nodes {
		if len(node.Choices) >= 2 {
			hasChoicePoint = true
		}
		if len(node.Choices) == 0 {
			if node.ID != packet.EntryNodeID && !hasNonEmptyString(node.RevisionNotes) {
				return fmt.Errorf("terminal seed node %q needs revision_notes for Writer continuation", node.ID)
			}
			continue
		}
		for _, choice := range node.Choices {
			targetID := strings.TrimSpace(choice.TargetNodeID)
			if targetID == "" {
				return fmt.Errorf("seed choice %q in node %q needs target_node_id", choice.ID, node.ID)
			}
			if targetID == node.ID {
				return fmt.Errorf("seed choice %q in node %q cannot target its own node", choice.ID, node.ID)
			}
			if _, err := packet.Node(targetID); err != nil {
				return fmt.Errorf("seed choice %q targets missing node %q", choice.ID, targetID)
			}
		}
	}
	if !hasChoicePoint {
		return errors.New("seed packet needs at least one playable choice point")
	}
	return nil
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func (b StoryBible) CreatedStoryText() string {
	var out []string
	out = append(out, b.Title, b.Premise, b.PlayerCharacter.Name, b.PlayerCharacter.Description, b.PlayerCharacter.Desire, b.PlayerCharacter.Wound, b.PlayerCharacter.Boundary, b.PlayerWorldConnection)
	out = append(out, b.WorldBasics...)
	out = append(out, b.StoryContextGuidance()...)
	for _, faction := range b.Factions {
		out = append(out, faction.ID, faction.Name, faction.Description)
	}
	out = append(out, b.CoreRelationships...)
	out = append(out, b.ToneGuidanceLines()...)
	out = append(out, b.FirstTurnExposition...)
	out = append(out, b.StoryArc...)
	out = append(out, b.CurrentArcPhase, b.NextRequiredRevelation, b.NextRequiredOpposition)
	out = append(out, b.SpentMotifs...)
	out = append(out, b.OpenThreads...)
	out = append(out, b.ResumeSummary)
	return strings.Join(out, "\n")
}

func CreatedPacketText(packet scene.StoryPacket) string {
	var out []string
	out = append(out, packet.ContextSnapshot.Location, packet.ContextSnapshot.Time)
	out = append(out, packet.ContextSnapshot.PresentActors...)
	for _, node := range packet.Nodes {
		out = append(out, node.Title)
		for _, beat := range node.Beats {
			out = append(out, beat.Text)
		}
		for _, choice := range node.Choices {
			out = append(out, choice.Label, choice.PlayerIntent)
		}
		for _, need := range node.ActorNeeds {
			out = append(out, need.ActorID, need.Reason)
			out = append(out, need.Targets...)
		}
		for _, intent := range node.MemoryIntents {
			out = append(out, intent.Summary, intent.PrivateToActor)
			out = append(out, intent.Witnesses...)
		}
	}
	return strings.Join(out, "\n")
}

func CollectionName(storyID string) string {
	return "sans_nom_story_" + strings.ReplaceAll(SanitizeStoryID(storyID), "-", "_")
}

func NowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
