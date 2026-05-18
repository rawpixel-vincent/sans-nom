package prompt

import (
	"context"
	"errors"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
)

func TestCodexRunnerRequestBodyUsesPriorityAndSchema(t *testing.T) {
	runner := CodexRunner{
		Model:       "gpt-5.5",
		Effort:      "medium",
		ServiceTier: "priority",
		SchemaName:  "StoryPacket",
		Schema:      StoryPacketSchema(),
	}

	body := runner.RequestBody("write packet")

	if body["model"] != "gpt-5.5" {
		t.Fatalf("model = %#v", body["model"])
	}
	if body["service_tier"] != "priority" {
		t.Fatalf("service_tier = %#v", body["service_tier"])
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "medium" {
		t.Fatalf("unexpected reasoning: %#v", body["reasoning"])
	}
	text, ok := body["text"].(map[string]any)
	if !ok {
		t.Fatalf("missing text format: %#v", body)
	}
	format, ok := text["format"].(map[string]any)
	if !ok || format["type"] != "json_schema" || format["name"] != "StoryPacket" || format["strict"] != true {
		t.Fatalf("unexpected schema format: %#v", text["format"])
	}
}

func TestSeedBundleSchemaContainsBibleAndPacket(t *testing.T) {
	schema := SeedBundleSchema()
	props := schema["properties"].(map[string]any)
	if props["bible"] == nil || props["packet"] == nil {
		t.Fatalf("missing seed bundle schema properties: %#v", props)
	}
	required := schema["required"].([]string)
	if len(required) != 2 || required[0] != "bible" || required[1] != "packet" {
		t.Fatalf("unexpected required fields: %#v", required)
	}
}

func TestCodexRunnerUsesLocalToolsWhenEnabled(t *testing.T) {
	client := &fakeLocalToolClient{}
	logger := gamelog.NewWriter(t.TempDir(), "story-1")
	runner := CodexRunner{
		Client:        client,
		Model:         "gpt-5.5",
		Effort:        "medium",
		ServiceTier:   "priority",
		LocalTools:    true,
		ToolRoot:      "/tmp/materials",
		MaxToolRounds: 7,
		Logger:        logger,
		ToolCategory:  "writer",
		CallerRole:    "writer",
		CallerSpeaker: "Writer",
		StoryID:       "story-1",
		Turn:          2,
		PacketID:      "packet-1",
		NodeID:        "node-1",
		RunID:         "run-1",
		ExtraTools: []codexoauth.LocalTool{{
			Name:        "custom_tool",
			Description: "custom",
			Parameters:  map[string]any{"type": "object"},
		}},
	}

	out, err := runner.Run(context.Background(), "read the manifest")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("output = %q", out)
	}
	if client.opts.Root != "/tmp/materials" || client.opts.MaxRounds != 7 {
		t.Fatalf("unexpected local tool opts: %#v", client.opts)
	}
	if len(client.opts.ExtraTools) != 1 || client.opts.ExtraTools[0].Name != "custom_tool" {
		t.Fatalf("unexpected extra tools: %#v", client.opts.ExtraTools)
	}
	if client.opts.Logger != logger || client.opts.ToolCategory != "writer" || client.opts.CallerRole != "writer" || client.opts.RunID != "run-1" {
		t.Fatalf("unexpected tool logging opts: %#v", client.opts)
	}
	if client.opts.ModelMetadata.Model != "gpt-5.5" || client.opts.ModelMetadata.ReasoningLevel != "medium" || client.opts.ModelMetadata.ServiceTier != "priority" {
		t.Fatalf("unexpected model metadata: %#v", client.opts.ModelMetadata)
	}
	if client.body["model"] != "gpt-5.5" {
		t.Fatalf("missing request body: %#v", client.body)
	}
}

type fakeLocalToolClient struct {
	body map[string]any
	opts codexoauth.LocalToolOptions
}

func (c *fakeLocalToolClient) Create(context.Context, map[string]any) (map[string]any, error) {
	return nil, errors.New("plain create should not be called")
}

func (c *fakeLocalToolClient) CreateWithLocalTools(_ context.Context, body map[string]any, opts codexoauth.LocalToolOptions) (map[string]any, error) {
	c.body = body
	c.opts = opts
	return map[string]any{"output_text": "ok"}, nil
}
