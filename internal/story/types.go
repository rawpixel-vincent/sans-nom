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
	OriginalLaws               []OriginalLaw     `json:"original_laws"`
	Factions                   []Faction         `json:"factions"`
	CoreRelationships          []string          `json:"core_relationships"`
	ToneDoctrine               []string          `json:"tone_doctrine"`
	SourceLoreInfluenceRefs    []string          `json:"source_lore_influence_refs"`
	ForbiddenLiteralCanonTerms []string          `json:"forbidden_literal_canon_terms"`
	FirstTurnExposition        []string          `json:"first_turn_exposition"`
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
	if len(b.OriginalLaws) == 0 {
		return errors.New("story bible needs at least one binding lore principle")
	}
	for _, law := range b.OriginalLaws {
		if strings.TrimSpace(law.ID) == "" || strings.TrimSpace(law.Name) == "" {
			return errors.New("each binding lore principle needs id and name")
		}
		if strings.TrimSpace(law.Description) == "" || strings.TrimSpace(law.Enforcement) == "" {
			return fmt.Errorf("binding lore principle %q needs description and enforcement", law.ID)
		}
		if strings.TrimSpace(law.SourcePattern) == "" {
			return fmt.Errorf("binding lore principle %q needs source pattern note", law.ID)
		}
		if strings.TrimSpace(law.WhyProfound) == "" || strings.TrimSpace(law.NotMerelyCruel) == "" {
			return fmt.Errorf("binding lore principle %q must explain why it has weight and is not merely cruel", law.ID)
		}
	}
	if len(b.FirstTurnExposition) < 5 {
		return fmt.Errorf("story bible needs at least 5 first-turn exposition requirements")
	}
	return nil
}

func ValidateSeedPacket(packet scene.StoryPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if len(packet.Nodes) != 5 {
		return fmt.Errorf("seed packet must have exactly 5 runway nodes, got %d", len(packet.Nodes))
	}
	entry, err := packet.EntryNode()
	if err != nil {
		return err
	}
	if len(entry.Beats) < 4 {
		return fmt.Errorf("turn_1_intro node needs at least 4 beats for story onboarding")
	}
	if len(entry.Choices) < 2 || len(entry.Choices) > 3 {
		return fmt.Errorf("turn_1_intro node needs 2-3 cosmetic choices, got %d", len(entry.Choices))
	}
	if err := validateSeedRunway(packet, entry); err != nil {
		return err
	}
	return nil
}

func validateSeedRunway(packet scene.StoryPacket, entry scene.SceneNode) error {
	decisionID := strings.TrimSpace(entry.Choices[0].TargetNodeID)
	if decisionID == "" {
		return fmt.Errorf("turn_1_intro choice %q needs target_node_id", entry.Choices[0].ID)
	}
	for _, choice := range entry.Choices {
		if strings.TrimSpace(choice.TargetNodeID) != decisionID {
			return fmt.Errorf("turn_1_intro choices must all target the same turn_2_decision node")
		}
	}
	if decisionID == entry.ID {
		return fmt.Errorf("turn_1_intro cannot target itself")
	}
	decision, err := packet.Node(decisionID)
	if err != nil {
		return fmt.Errorf("turn_2_decision node %q not found", decisionID)
	}
	if len(decision.Choices) != 3 {
		return fmt.Errorf("turn_2_decision node %q needs exactly 3 real choices, got %d", decision.ID, len(decision.Choices))
	}
	branchTargets := map[string]struct{}{}
	for _, choice := range decision.Choices {
		targetID := strings.TrimSpace(choice.TargetNodeID)
		if targetID == "" {
			return fmt.Errorf("turn_2_decision choice %q needs target_node_id", choice.ID)
		}
		if targetID == entry.ID || targetID == decision.ID {
			return fmt.Errorf("turn_2_decision choice %q must target a turn_3 branch node", choice.ID)
		}
		if _, ok := branchTargets[targetID]; ok {
			return fmt.Errorf("turn_2_decision choices must target three distinct turn_3 branch nodes")
		}
		branchTargets[targetID] = struct{}{}
		branch, err := packet.Node(targetID)
		if err != nil {
			return fmt.Errorf("turn_3 branch node %q not found", targetID)
		}
		if len(branch.Choices) != 0 {
			return fmt.Errorf("turn_3 branch node %q must be terminal with no choices", branch.ID)
		}
		if !hasNonEmptyString(branch.RevisionNotes) {
			return fmt.Errorf("turn_3 branch node %q needs revision_notes requiring Writer continuation", branch.ID)
		}
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
	for _, law := range b.OriginalLaws {
		out = append(out, law.ID, law.Name, law.Description, law.Enforcement, law.WhyProfound, law.NotMerelyCruel)
	}
	for _, faction := range b.Factions {
		out = append(out, faction.ID, faction.Name, faction.Description)
	}
	out = append(out, b.CoreRelationships...)
	out = append(out, b.ToneDoctrine...)
	out = append(out, b.FirstTurnExposition...)
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

func ForbiddenCanonTerms() []string {
	return []string{
		"aghora",
		"aghnar",
		"bushitaka",
		"castaka",
		"council of castaka",
		"epyphite",
		"honorata",
		"lothar",
		"meta-baron",
		"metabaron",
		"metabunker",
		"oda",
		"othon",
		"roundtable",
		"shabda-oud",
		"steelhead",
		"tonto",
		"vicenta",
	}
}

func CollectionName(storyID string) string {
	return "sans_nom_story_" + strings.ReplaceAll(SanitizeStoryID(storyID), "-", "_")
}

func NowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
