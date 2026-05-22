package prompt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

type CodexClient interface {
	Create(ctx context.Context, body map[string]any) (map[string]any, error)
}

type LocalToolCodexClient interface {
	CodexClient
	CreateWithLocalTools(ctx context.Context, body map[string]any, opts codexoauth.LocalToolOptions) (map[string]any, error)
}

type CodexRunner struct {
	Client           CodexClient
	Model            string
	Effort           string
	ServiceTier      string
	AuthPath         string
	SystemPrompt     string
	Timeout          time.Duration
	SchemaName       string
	Schema           map[string]any
	LocalTools       bool
	ToolRoot         string
	ToolVisiblePaths []string
	MaxToolRounds    int
	ExtraTools       []codexoauth.LocalTool
	Logger           *gamelog.Writer
	ToolCategory     string
	CallerRole       string
	CallerSpeaker    string
	StoryID          string
	Turn             int
	PacketID         string
	NodeID           string
	RunID            string
}

func (r CodexRunner) Run(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("prompt text is required")
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := r.Client
	if client == nil {
		created, err := codexoauth.NewClient(codexoauth.Options{
			AuthPath:      r.AuthPath,
			ModelOverride: r.model(),
			SystemPrompt:  r.SystemPrompt,
		})
		if err != nil {
			return "", err
		}
		client = created
	}
	var resp map[string]any
	var err error
	body := r.RequestBody(text)
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if r.LocalTools {
			toolClient, ok := client.(LocalToolCodexClient)
			if !ok {
				return "", errors.New("codex runner local tools require a client with CreateWithLocalTools")
			}
			resp, err = toolClient.CreateWithLocalTools(ctx, body, r.localToolOptions())
		} else {
			resp, err = client.Create(ctx, body)
		}
		if err == nil || !retryableCodexError(err) {
			break
		}
		if attempt < maxAttempts-1 {
			select {
			case <-time.After(time.Duration(attempt+1) * 750 * time.Millisecond):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	if err != nil {
		return "", err
	}
	output, _ := resp["output_text"].(string)
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("codex response had empty output_text")
	}
	return output, nil
}

func retryableCodexError(err error) bool {
	return IsRetryableCodexError(err)
}

func IsRetryableCodexError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, pattern := range []string{
		"internal_error",
		"stream error",
		"unexpected eof",
		"can't assign requested address",
		"connection reset by peer",
		"broken pipe",
		"i/o timeout",
		"operation timed out",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"server disconnected",
	} {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

func (r CodexRunner) RequestBody(text string) map[string]any {
	body := map[string]any{
		"model": r.model(),
		"input": []any{map[string]any{
			"role":    "user",
			"content": text,
		}},
		"reasoning": map[string]any{
			"effort": r.effort(),
		},
		"service_tier": r.serviceTier(),
	}
	if r.SchemaName != "" && r.Schema != nil {
		body["text"] = map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   r.SchemaName,
				"schema": r.Schema,
				"strict": true,
			},
		}
	}
	return body
}

func (r CodexRunner) ModelMetadata() modelmeta.Metadata {
	return modelmeta.Metadata{
		ProviderType:   "codex_oauth",
		ModelProvider:  "openai",
		Model:          r.model(),
		ReasoningLevel: r.effort(),
		ServiceTier:    r.serviceTier(),
	}
}

func (r CodexRunner) localToolOptions() codexoauth.LocalToolOptions {
	root := r.ToolRoot
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return codexoauth.LocalToolOptions{
		Root:          root,
		MaxRounds:     r.MaxToolRounds,
		ExtraTools:    r.ExtraTools,
		VisiblePaths:  r.ToolVisiblePaths,
		Logger:        r.Logger,
		ToolCategory:  r.ToolCategory,
		CallerRole:    r.CallerRole,
		CallerSpeaker: r.CallerSpeaker,
		StoryID:       r.StoryID,
		Turn:          r.Turn,
		PacketID:      r.PacketID,
		NodeID:        r.NodeID,
		RunID:         r.RunID,
		ModelMetadata: r.ModelMetadata(),
	}
}

func (r CodexRunner) model() string {
	if r.Model != "" {
		return r.Model
	}
	return "gpt-5.5"
}

func (r CodexRunner) effort() string {
	if r.Effort != "" {
		return r.Effort
	}
	return "medium"
}

func (r CodexRunner) serviceTier() string {
	if r.ServiceTier != "" {
		return r.ServiceTier
	}
	if tier := strings.TrimSpace(os.Getenv("SANS_NOM_CODEX_OAUTH_SERVICE_TIER")); tier != "" {
		return tier
	}
	return "priority"
}

func StoryPacketSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "entry_node_id", "context_snapshot", "nodes", "revision_notes", "invalidation_notes", "metadata"},
		"properties": map[string]any{
			"id":               map[string]any{"type": "string"},
			"entry_node_id":    map[string]any{"type": "string"},
			"context_snapshot": contextSnapshotSchema(),
			"nodes": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 5,
				"items":    sceneNodeSchema(),
			},
			"revision_notes":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"invalidation_notes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"metadata":           packetMetadataSchema(),
		},
	}
}

func SeedBundleSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"bible", "packet"},
		"properties": map[string]any{
			"bible":  storyBibleSchema(),
			"packet": StoryPacketSchema(),
		},
	}
}

func NarratorDraftSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"prose", "surfaced_actor_messages", "choices"},
		"properties": map[string]any{
			"prose": map[string]any{"type": "string"},
			"surfaced_actor_messages": map[string]any{
				"type":  "array",
				"items": actorMessageSchema(),
			},
			"choices": map[string]any{
				"type":  "array",
				"items": choiceSchema(),
			},
		},
	}
}

func storyBibleSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"story_id",
			"title",
			"premise",
			"player_character",
			"player_world_connection",
			"world_basics",
			"lore_principles",
			"factions",
			"core_relationships",
			"tone_doctrine",
			"first_turn_exposition",
			"story_arc",
			"current_arc_phase",
			"next_required_revelation",
			"next_required_opposition",
			"spent_motifs",
			"open_threads",
			"resume_summary",
			"metadata",
		},
		"properties": map[string]any{
			"story_id":                 map[string]any{"type": "string"},
			"title":                    map[string]any{"type": "string"},
			"premise":                  map[string]any{"type": "string"},
			"player_character":         playerCharacterSchema(),
			"player_world_connection":  map[string]any{"type": "string"},
			"world_basics":             stringArraySchema(3),
			"lore_principles":          map[string]any{"type": "array", "minItems": 1, "items": originalLawSchema()},
			"factions":                 map[string]any{"type": "array", "items": factionSchema()},
			"core_relationships":       stringArraySchema(0),
			"tone_doctrine":            stringArraySchema(0),
			"first_turn_exposition":    stringArraySchema(5),
			"story_arc":                stringArraySchema(4),
			"current_arc_phase":        map[string]any{"type": "string"},
			"next_required_revelation": map[string]any{"type": "string"},
			"next_required_opposition": map[string]any{"type": "string"},
			"spent_motifs":             stringArraySchema(0),
			"open_threads":             stringArraySchema(0),
			"resume_summary":           map[string]any{"type": "string"},
			"metadata":                 seedResumeMetadataSchema(),
		},
	}
}

func playerCharacterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"peer_id", "name", "description", "desire", "wound", "boundary"},
		"properties": map[string]any{
			"peer_id":     map[string]any{"type": "string"},
			"name":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"desire":      map[string]any{"type": "string"},
			"wound":       map[string]any{"type": "string"},
			"boundary":    map[string]any{"type": "string"},
		},
	}
}

func originalLawSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "name", "description", "source_pattern", "enforcement", "why_profound", "not_merely_cruel"},
		"properties": map[string]any{
			"id":               map[string]any{"type": "string"},
			"name":             map[string]any{"type": "string"},
			"description":      map[string]any{"type": "string"},
			"source_pattern":   map[string]any{"type": "string"},
			"enforcement":      map[string]any{"type": "string"},
			"why_profound":     map[string]any{"type": "string"},
			"not_merely_cruel": map[string]any{"type": "string"},
		},
	}
}

func factionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "name", "description"},
		"properties": map[string]any{
			"id":          map[string]any{"type": "string"},
			"name":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
		},
	}
}

func stringArraySchema(minItems int) map[string]any {
	schema := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	if minItems > 0 {
		schema["minItems"] = minItems
	}
	return schema
}

func ActorOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"interior", "dialogue", "action"},
		"properties": map[string]any{
			"interior": map[string]any{"type": "string"},
			"dialogue": map[string]any{"type": "string"},
			"action":   map[string]any{"type": "string"},
		},
	}
}

func SeedReviewSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"accepted", "blocking_issues", "revision_instructions"},
		"properties": map[string]any{
			"accepted":              map[string]any{"type": "boolean"},
			"blocking_issues":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"revision_instructions": map[string]any{"type": "string"},
		},
	}
}

func sceneNodeSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "title", "beats", "gates", "choices", "actor_needs", "memory_intents", "revision_notes", "invalidation_notes", "writer_notes"},
		"properties": map[string]any{
			"id":      map[string]any{"type": "string"},
			"title":   map[string]any{"type": "string"},
			"beats":   map[string]any{"type": "array", "minItems": 1, "items": beatSchema()},
			"gates":   map[string]any{"type": "array", "items": gateSchema()},
			"choices": map[string]any{"type": "array", "items": choiceSchema()},
			"actor_needs": map[string]any{
				"type":     "array",
				"maxItems": 4,
				"items":    actorNeedSchema(),
			},
			"memory_intents":     map[string]any{"type": "array", "items": memoryIntentSchema()},
			"revision_notes":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"invalidation_notes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"writer_notes":       map[string]any{"type": "string"},
		},
	}
}

func contextSnapshotSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"workspace_id", "session_id", "location", "time", "latest_narrator_prose", "last_user_decision", "present_actors", "representations", "relevant_lore_snippets"},
		"properties": map[string]any{
			"workspace_id":           map[string]any{"type": "string"},
			"session_id":             map[string]any{"type": "string"},
			"location":               map[string]any{"type": "string"},
			"time":                   map[string]any{"type": "string"},
			"latest_narrator_prose":  map[string]any{"type": "string"},
			"last_user_decision":     map[string]any{"type": "string"},
			"present_actors":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"representations":        emptyObjectSchema(),
			"relevant_lore_snippets": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
		"required":             []string{},
	}
}

func packetMetadataSchema() map[string]any {
	required := []string{"current_arc_phase", "next_arc_phase", "spent_motifs"}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties": map[string]any{
			"current_arc_phase": map[string]any{"type": "string"},
			"next_arc_phase":    map[string]any{"type": "string"},
			"spent_motifs":      map[string]any{"type": "string"},
		},
	}
}

func seedResumeMetadataSchema() map[string]any {
	properties := map[string]any{
		"seed_version":        map[string]any{"type": "string"},
		"generated_at":        map[string]any{"type": "string"},
		"resume_point":        map[string]any{"type": "string"},
		"current_checkpoint":  map[string]any{"type": "string"},
		"continuation_hooks":  map[string]any{"type": "string"},
		"canon_validated":     map[string]any{"type": "string"},
		"open_threads":        map[string]any{"type": "string"},
		"gameplay_mode":       map[string]any{"type": "string"},
		"ground_lore_pack_id": map[string]any{"type": "string"},
	}
	required := []string{
		"seed_version",
		"generated_at",
		"resume_point",
		"current_checkpoint",
		"continuation_hooks",
		"canon_validated",
		"open_threads",
		"gameplay_mode",
		"ground_lore_pack_id",
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

func beatSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "text"},
		"properties": map[string]any{
			"id":   map[string]any{"type": "string"},
			"text": map[string]any{"type": "string"},
		},
	}
}

func choiceSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "label", "target_node_id", "player_intent"},
		"properties": map[string]any{
			"id":             map[string]any{"type": "string"},
			"label":          map[string]any{"type": "string"},
			"target_node_id": map[string]any{"type": "string"},
			"player_intent":  map[string]any{"type": "string"},
		},
	}
}

func gateSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "condition", "failure_node_id"},
		"properties": map[string]any{
			"id":              map[string]any{"type": "string"},
			"condition":       map[string]any{"type": "string"},
			"failure_node_id": map[string]any{"type": "string"},
		},
	}
}

func actorNeedSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"actor_id", "mode", "reason", "targets"},
		"properties": map[string]any{
			"actor_id": map[string]any{"type": "string"},
			"mode":     map[string]any{"type": "string", "enum": []string{"present", "remembered", "narrated", "valid_scene_context"}},
			"reason":   map[string]any{"type": "string"},
			"targets":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
}

func memoryIntentSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"id", "summary", "policy", "witnesses", "private_to_actor"},
		"properties": map[string]any{
			"id":               map[string]any{"type": "string"},
			"summary":          map[string]any{"type": "string"},
			"policy":           map[string]any{"type": "string", "enum": []string{"rolled_out_canon", "actor_private", "planning_only"}},
			"witnesses":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"private_to_actor": map[string]any{"type": "string"},
		},
	}
}

func actorMessageSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"actor_id", "surface", "dialogue", "action", "witnesses"},
		"properties": map[string]any{
			"actor_id":  map[string]any{"type": "string"},
			"surface":   map[string]any{"type": "string"},
			"dialogue":  map[string]any{"type": "string"},
			"action":    map[string]any{"type": "string"},
			"witnesses": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
}
