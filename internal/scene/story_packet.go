package scene

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

const (
	ActorModePresent    = "present"
	ActorModeRemembered = "remembered"
	ActorModeNarrated   = "narrated"
	ActorModeContext    = "valid_scene_context"

	MemoryPolicyCanon        = "rolled_out_canon"
	MemoryPolicyActorPrivate = "actor_private"
	MemoryPolicyPlanning     = "planning_only"

	CommitSourcePlayer       = "player"
	CommitSourceNarrator     = "narrator"
	CommitSourceActorSurface = "actor_surface"
	CommitSourceActorPrivate = "actor_private"
	CommitSourceWriterDraft  = "writer_draft"
	CommitSourceUnusedBranch = "unused_branch"
)

// StoryPacket is the bounded tree the Creative Writer prepares in the
// background. It is not canon. Only a Narrator rollout may become canon.
type StoryPacket struct {
	ID                string            `json:"id"`
	EntryNodeID       string            `json:"entry_node_id"`
	ContextSnapshot   ContextSnapshot   `json:"context_snapshot"`
	Nodes             []SceneNode       `json:"nodes"`
	RevisionNotes     []string          `json:"revision_notes,omitempty"`
	InvalidationNotes []string          `json:"invalidation_notes,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type MissingChoiceTargetError struct {
	NodeID       string
	ChoiceID     string
	TargetNodeID string
	KnownNodeIDs []string
}

func (e MissingChoiceTargetError) Error() string {
	return fmt.Sprintf("missing target node inside bounded packet: scene node %q choice %q targets missing node %q", e.NodeID, e.ChoiceID, e.TargetNodeID)
}

type ContextSnapshot struct {
	WorkspaceID          string            `json:"workspace_id"`
	SessionID            string            `json:"session_id"`
	Location             string            `json:"location,omitempty"`
	Time                 string            `json:"time,omitempty"`
	LatestNarratorProse  string            `json:"latest_narrator_prose,omitempty"`
	LastUserDecision     string            `json:"last_user_decision,omitempty"`
	PresentActors        []string          `json:"present_actors,omitempty"`
	Representations      map[string]string `json:"representations,omitempty"`
	RelevantLoreSnippets []string          `json:"relevant_lore_snippets,omitempty"`
}

type SceneNode struct {
	ID                string         `json:"id"`
	Title             string         `json:"title,omitempty"`
	Beats             []Beat         `json:"beats"`
	Gates             []Gate         `json:"gates,omitempty"`
	Choices           []Choice       `json:"choices"`
	ActorNeeds        []ActorNeed    `json:"actor_needs,omitempty"`
	MemoryIntents     []MemoryIntent `json:"memory_intents,omitempty"`
	RevisionNotes     []string       `json:"revision_notes,omitempty"`
	InvalidationNotes []string       `json:"invalidation_notes,omitempty"`
	WriterNotes       string         `json:"writer_notes,omitempty"`
}

type Beat struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text"`
}

type Gate struct {
	ID            string `json:"id"`
	Condition     string `json:"condition"`
	FailureNodeID string `json:"failure_node_id,omitempty"`
}

type Choice struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	TargetNodeID string `json:"target_node_id,omitempty"`
	PlayerIntent string `json:"player_intent,omitempty"`
}

type ActorNeed struct {
	ActorID string   `json:"actor_id"`
	Mode    string   `json:"mode"`
	Reason  string   `json:"reason,omitempty"`
	Targets []string `json:"targets,omitempty"`
}

type MemoryIntent struct {
	ID             string   `json:"id,omitempty"`
	Summary        string   `json:"summary"`
	Policy         string   `json:"policy"`
	Witnesses      []string `json:"witnesses,omitempty"`
	PrivateToActor string   `json:"private_to_actor,omitempty"`
}

type PlayerDecision struct {
	ChoiceID string `json:"choice_id,omitempty"`
	FreeText string `json:"free_text,omitempty"`
}

type ActorMessage struct {
	ActorID   string   `json:"actor_id"`
	Interior  string   `json:"interior,omitempty"`
	Dialogue  string   `json:"dialogue,omitempty"`
	Action    string   `json:"action,omitempty"`
	Surface   string   `json:"surface,omitempty"`
	Witnesses []string `json:"witnesses,omitempty"`
}

type CanonEvent struct {
	PeerID    string   `json:"peer_id"`
	Content   string   `json:"content"`
	Source    string   `json:"source"`
	Witnesses []string `json:"witnesses,omitempty"`
}

type Commit struct {
	WorkspaceID    string         `json:"workspace_id"`
	SessionID      string         `json:"session_id"`
	PacketID       string         `json:"packet_id"`
	NodeID         string         `json:"node_id"`
	PlayerDecision PlayerDecision `json:"player_decision"`
	FinalProse     string         `json:"final_prose"`
	ActorMessages  []ActorMessage `json:"actor_messages,omitempty"`
	Events         []CanonEvent   `json:"events"`
	Witnesses      []string       `json:"witnesses"`
}

func DecodeStoryPacket(r io.Reader) (StoryPacket, error) {
	var packet StoryPacket
	if err := json.NewDecoder(r).Decode(&packet); err != nil {
		return StoryPacket{}, err
	}
	return packet, packet.Validate()
}

func (p StoryPacket) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("story packet id is required")
	}
	if len(p.Nodes) == 0 {
		return errors.New("story packet needs at least one node")
	}
	if len(p.Nodes) > 5 {
		return fmt.Errorf("story packet has %d nodes; phase 0 packets are capped at 5", len(p.Nodes))
	}

	nodeIDs := map[string]struct{}{}
	for _, node := range p.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return errors.New("scene node id is required")
		}
		if _, ok := nodeIDs[node.ID]; ok {
			return fmt.Errorf("duplicate scene node id %q", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
		if len(node.Beats) == 0 {
			return fmt.Errorf("scene node %q needs at least one beat", node.ID)
		}
		for _, beat := range node.Beats {
			if strings.TrimSpace(beat.Text) == "" {
				return fmt.Errorf("scene node %q has an empty beat", node.ID)
			}
		}
		if err := validateActorNeeds(node); err != nil {
			return fmt.Errorf("scene node %q: %w", node.ID, err)
		}
		for _, intent := range node.MemoryIntents {
			if err := intent.Validate(); err != nil {
				return fmt.Errorf("scene node %q: %w", node.ID, err)
			}
		}
	}

	entry := p.EntryNodeID
	if entry == "" {
		entry = p.Nodes[0].ID
	}
	if _, ok := nodeIDs[entry]; !ok {
		return fmt.Errorf("entry node %q not found", entry)
	}
	for _, node := range p.Nodes {
		for _, choice := range node.Choices {
			if strings.TrimSpace(choice.ID) == "" {
				return fmt.Errorf("scene node %q has a choice without id", node.ID)
			}
			if strings.TrimSpace(choice.Label) == "" {
				return fmt.Errorf("scene node %q choice %q needs label", node.ID, choice.ID)
			}
			if choice.TargetNodeID != "" {
				if _, ok := nodeIDs[choice.TargetNodeID]; !ok {
					return &MissingChoiceTargetError{
						NodeID:       node.ID,
						ChoiceID:     choice.ID,
						TargetNodeID: choice.TargetNodeID,
						KnownNodeIDs: sortedNodeIDs(nodeIDs),
					}
				}
			}
		}
	}
	return nil
}

func sortedNodeIDs(nodeIDs map[string]struct{}) []string {
	out := make([]string, 0, len(nodeIDs))
	for id := range nodeIDs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func validateActorNeeds(node SceneNode) error {
	validModes := []string{ActorModePresent, ActorModeRemembered, ActorModeNarrated, ActorModeContext}
	seen := map[string]struct{}{}
	for _, need := range node.ActorNeeds {
		if strings.TrimSpace(need.ActorID) == "" {
			return errors.New("actor need without actor_id")
		}
		if !slices.Contains(validModes, need.Mode) {
			return fmt.Errorf("actor need %q has invalid mode %q", need.ActorID, need.Mode)
		}
		key := need.ActorID + "\x00" + need.Mode
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate actor need %q", need.ActorID)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (m MemoryIntent) Validate() error {
	if strings.TrimSpace(m.Summary) == "" {
		return errors.New("memory intent summary is required")
	}
	switch m.Policy {
	case MemoryPolicyCanon:
		if len(m.Witnesses) == 0 {
			return fmt.Errorf("rolled-out canon memory %q needs witnesses", m.Summary)
		}
	case MemoryPolicyActorPrivate:
		if strings.TrimSpace(m.PrivateToActor) == "" {
			return fmt.Errorf("actor-private memory %q needs private_to_actor", m.Summary)
		}
	case MemoryPolicyPlanning:
		return nil
	default:
		return fmt.Errorf("memory intent %q has invalid policy %q", m.Summary, m.Policy)
	}
	return nil
}

func (p StoryPacket) EntryNode() (SceneNode, error) {
	entry := p.EntryNodeID
	if entry == "" && len(p.Nodes) > 0 {
		entry = p.Nodes[0].ID
	}
	return p.Node(entry)
}

func (p StoryPacket) Node(id string) (SceneNode, error) {
	for _, node := range p.Nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return SceneNode{}, fmt.Errorf("scene node %q not found", id)
}

func (n SceneNode) Choice(id string) (Choice, error) {
	for _, choice := range n.Choices {
		if choice.ID == id {
			return choice, nil
		}
	}
	return Choice{}, fmt.Errorf("choice %q not found in node %q", id, n.ID)
}

func (n SceneNode) CanonMemoryIntents() []MemoryIntent {
	var out []MemoryIntent
	for _, intent := range n.MemoryIntents {
		if intent.Policy == MemoryPolicyCanon {
			out = append(out, intent)
		}
	}
	return out
}

func (c Commit) ValidateCanonOnly() error {
	if strings.TrimSpace(c.FinalProse) == "" {
		return errors.New("final prose is required")
	}
	for _, event := range c.Events {
		if event.Source == CommitSourceWriterDraft || event.Source == CommitSourceUnusedBranch {
			return fmt.Errorf("planning artifact %q cannot be committed to Honcho", event.Source)
		}
		if event.Source == CommitSourceActorPrivate {
			return fmt.Errorf("actor-private event for %q cannot be committed to shared scene memory", event.PeerID)
		}
		if len(event.Witnesses) == 0 {
			return fmt.Errorf("canon event %q needs directional witnesses", event.Content)
		}
	}
	for _, msg := range c.ActorMessages {
		if msg.Interior != "" && msg.Surface == "" {
			continue
		}
		if msg.Surface != "" && len(msg.Witnesses) == 0 {
			return fmt.Errorf("surfaced actor message for %q needs witnesses", msg.ActorID)
		}
	}
	return nil
}
