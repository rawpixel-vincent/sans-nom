package contextbundle

import (
	"context"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

type Bridge interface {
	SceneContext(ctx context.Context, req bridgeclient.SceneContextRequest) (bridgeclient.SceneContextResponse, error)
	Representation(ctx context.Context, req bridgeclient.RepresentationRequest) (bridgeclient.RepresentationResponse, error)
	GetCard(ctx context.Context, workspaceID, peerID, target string) ([]string, error)
}

type CardProvider interface {
	LocalCard(actorID string) (Card, bool)
	ActorIDs() []string
}

type Assembler struct {
	Lore    lore.Retriever
	Bridge  Bridge
	Cards   CardProvider
	Options Options
}

type Options struct {
	PlayerPeerID           string
	RecentSceneCount       int
	NarratorLoreK          int
	WriterLoreK            int
	MaxRecentSceneChars    int
	MaxLoreChars           int
	MaxRepresentationChars int
	MaxActorCardFacts      int
	MaxActorTargets        int
	MaxActorManifestItems  int
	NarratorSoftCap        int
	WriterSoftCap          int
	RequireBridge          bool
	RequireLore            bool
	SkipMissingActorCards  bool
}

type BuildRequest struct {
	Packet   scene.StoryPacket    `json:"packet"`
	NodeID   string               `json:"node_id,omitempty"`
	Decision scene.PlayerDecision `json:"player_decision,omitempty"`
	Bible    *story.StoryBible    `json:"story_bible,omitempty"`
}

type ContextBundleSummary struct {
	Role         string   `json:"role"`
	PacketID     string   `json:"packet_id"`
	NodeID       string   `json:"node_id"`
	ActorCount   int      `json:"actor_count"`
	LoreHitCount int      `json:"lore_hit_count"`
	Warnings     []string `json:"warnings,omitempty"`
}

type SceneState struct {
	WorkspaceID   string   `json:"workspace_id"`
	SessionID     string   `json:"session_id"`
	Location      string   `json:"location,omitempty"`
	Time          string   `json:"time,omitempty"`
	PresentActors []string `json:"present_actors,omitempty"`
}

type LoreHit struct {
	ID          string  `json:"id"`
	Text        string  `json:"text"`
	Source      string  `json:"source"`
	Score       float64 `json:"score"`
	Kind        string  `json:"kind,omitempty"`
	PageID      string  `json:"page_id,omitempty"`
	PeerID      string  `json:"peer_id,omitempty"`
	CharacterID string  `json:"character_id,omitempty"`
	Volume      *int    `json:"volume,omitempty"`
}

type Representation struct {
	Observer string `json:"observer"`
	Observed string `json:"observed"`
	Text     string `json:"text"`
}

type ActorContext struct {
	ActorID         string           `json:"actor_id"`
	Mode            string           `json:"mode"`
	Reason          string           `json:"reason,omitempty"`
	Targets         []string         `json:"targets,omitempty"`
	Card            string           `json:"card"`
	CardSource      string           `json:"card_source"`
	Representations []Representation `json:"representations,omitempty"`
	LoreHits        []LoreHit        `json:"lore_hits,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type PublicSceneNode struct {
	ID            string               `json:"id"`
	Title         string               `json:"title,omitempty"`
	Beats         []scene.Beat         `json:"beats"`
	Gates         []scene.Gate         `json:"gates,omitempty"`
	Choices       []scene.Choice       `json:"choices"`
	ActorNeeds    []scene.ActorNeed    `json:"actor_needs,omitempty"`
	MemoryIntents []scene.MemoryIntent `json:"memory_intents,omitempty"`
}

type NarratorContext struct {
	Summary         ContextBundleSummary `json:"summary"`
	MaxRenderChars  int                  `json:"max_render_chars,omitempty"`
	StoryBible      *story.StoryBible    `json:"story_bible,omitempty"`
	SceneState      SceneState           `json:"scene_state"`
	PlayerDecision  scene.PlayerDecision `json:"player_decision,omitempty"`
	SelectedNode    PublicSceneNode      `json:"selected_node"`
	ActorContexts   []ActorContext       `json:"actor_contexts,omitempty"`
	LoreHits        []LoreHit            `json:"lore_hits,omitempty"`
	RecentScenes    []string             `json:"recent_scenes,omitempty"`
	Representations []Representation     `json:"representations,omitempty"`
	WitnessRules    []string             `json:"witness_rules"`
	Warnings        []string             `json:"warnings,omitempty"`
}

type WriterContext struct {
	Summary         ContextBundleSummary `json:"summary"`
	MaxRenderChars  int                  `json:"max_render_chars,omitempty"`
	StoryBible      *story.StoryBible    `json:"story_bible,omitempty"`
	SceneState      SceneState           `json:"scene_state"`
	PlayerDecision  scene.PlayerDecision `json:"player_decision,omitempty"`
	SelectedNodeID  string               `json:"selected_node_id"`
	Packet          scene.StoryPacket    `json:"packet"`
	RecentScenes    []string             `json:"recent_scenes,omitempty"`
	Representations []Representation     `json:"representations,omitempty"`
	LoreByKind      map[string][]LoreHit `json:"lore_by_kind,omitempty"`
	ActorManifest   []ActorManifestItem  `json:"actor_manifest,omitempty"`
	Warnings        []string             `json:"warnings,omitempty"`
}

type ActorManifestItem struct {
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role,omitempty"`
	Source      string `json:"source,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Explicit    bool   `json:"explicit,omitempty"`
}
