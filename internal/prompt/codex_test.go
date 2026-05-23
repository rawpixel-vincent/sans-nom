package prompt

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestStoryPacketSchemaRequiresClosedProgressionMetadata(t *testing.T) {
	schema := StoryPacketSchema()
	props := schema["properties"].(map[string]any)
	metadata, ok := props["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("packet schema missing metadata object: %#v", props["metadata"])
	}
	if metadata["additionalProperties"] != false {
		t.Fatalf("packet metadata must be closed for strict response schemas: %#v", metadata)
	}
	metadataProps := metadata["properties"].(map[string]any)
	metadataRequired := metadata["required"].([]string)
	for _, field := range []string{"current_arc_phase", "next_arc_phase", "spent_motifs"} {
		if metadataProps[field] == nil {
			t.Fatalf("packet metadata schema missing %q: %#v", field, metadataProps)
		}
		if !containsString(metadataRequired, field) {
			t.Fatalf("packet metadata schema should require %q: %#v", field, metadataRequired)
		}
	}
}

func TestResponseSchemasUseStrictClosedObjects(t *testing.T) {
	for name, schema := range map[string]map[string]any{
		"ActorOutput":   ActorOutputSchema(),
		"NarratorDraft": NarratorDraftSchema(),
		"SeedBundle":    SeedBundleSchema(),
		"SeedReview":    SeedReviewSchema(),
		"StoryPacket":   StoryPacketSchema(),
	} {
		assertStrictObjectSchema(t, name, schema)
	}
}

func TestSeedBundleSchemaUsesFaithfulLorePrinciples(t *testing.T) {
	schema := SeedBundleSchema()
	props := schema["properties"].(map[string]any)
	bible := props["bible"].(map[string]any)
	bibleProps := bible["properties"].(map[string]any)
	if bibleProps["lore_principles"] == nil {
		t.Fatalf("seed bible schema missing lore_principles: %#v", bibleProps)
	}
	if bibleProps["story_arc"] == nil {
		t.Fatalf("seed bible schema missing story_arc: %#v", bibleProps)
	}
	metadata, ok := bibleProps["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("seed bible schema missing metadata object: %#v", bibleProps["metadata"])
	}
	metadataProps := metadata["properties"].(map[string]any)
	metadataRequired := metadata["required"].([]string)
	for _, field := range []string{"seed_version", "generated_at", "resume_point", "current_checkpoint", "continuation_hooks", "canon_validated", "open_threads", "gameplay_mode", "ground_lore_pack_id"} {
		if metadataProps[field] == nil {
			t.Fatalf("seed metadata schema missing %q: %#v", field, metadataProps)
		}
		if !containsString(metadataRequired, field) {
			t.Fatalf("seed metadata schema should require %q: %#v", field, metadataRequired)
		}
	}
	required := bible["required"].([]string)
	if !containsString(required, "story_arc") {
		t.Fatalf("seed bible schema should require story_arc: %#v", required)
	}
	for field := range bibleProps {
		if !containsString(required, field) {
			t.Fatalf("strict seed bible schema property %q must also be required: %#v", field, required)
		}
	}
	for _, legacy := range []string{"original_laws", "source_lore_influence_refs", "forbidden_literal_canon_terms"} {
		if bibleProps[legacy] != nil {
			t.Fatalf("seed bible schema should not prompt legacy field %q: %#v", legacy, bibleProps)
		}
	}
	for _, legacy := range []string{"original_laws", "source_lore_influence_refs", "forbidden_literal_canon_terms"} {
		for _, field := range required {
			if field == legacy {
				t.Fatalf("seed bible schema should not require legacy field %q: %#v", legacy, required)
			}
		}
	}
}

func TestCodexRunnerUsesLocalToolsWhenEnabled(t *testing.T) {
	client := &fakeLocalToolClient{}
	logger := gamelog.NewWriter(t.TempDir(), "story-1")
	runner := CodexRunner{
		Client:           client,
		Model:            "gpt-5.5",
		Effort:           "medium",
		ServiceTier:      "priority",
		LocalTools:       true,
		ToolRoot:         "/tmp/materials",
		ToolVisiblePaths: []string{"story", "scenarists"},
		MaxToolRounds:    7,
		Logger:           logger,
		ToolCategory:     "writer",
		CallerRole:       "writer",
		CallerSpeaker:    "Writer",
		StoryID:          "story-1",
		Turn:             2,
		PacketID:         "packet-1",
		NodeID:           "node-1",
		RunID:            "run-1",
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
	if len(client.opts.VisiblePaths) != 2 || client.opts.VisiblePaths[0] != "story" || client.opts.VisiblePaths[1] != "scenarists" {
		t.Fatalf("unexpected visible paths: %#v", client.opts.VisiblePaths)
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

func TestCodexRunnerRetriesTransientNetworkFailure(t *testing.T) {
	client := &retryClient{
		errs: []error{
			errors.New("read tcp 192.168.1.39:64645->172.64.155.209:443: read: can't assign requested address"),
		},
	}
	runner := CodexRunner{
		Client: client,
		Model:  "gpt-5.5",
		Effort: "medium",
	}

	out, err := runner.Run(context.Background(), "verify seed")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("output = %q", out)
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d", client.calls)
	}
}

func TestCodexRunnerLogsDirectProviderLifecycle(t *testing.T) {
	client := &retryClient{}
	logger := gamelog.NewWriter(t.TempDir(), "story-1")
	runner := CodexRunner{
		Client:        client,
		Model:         "gpt-5.5",
		Effort:        "medium",
		ServiceTier:   "priority",
		SchemaName:    "SeedReview",
		Logger:        logger,
		ToolCategory:  "model",
		CallerRole:    "nerd-verifier",
		CallerSpeaker: "Nerd Verifier",
	}

	out, err := runner.Run(context.Background(), "verify seed")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("output = %q", out)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Event != "model.provider.request" || events[0].Fields["status"] != "pending" {
		t.Fatalf("unexpected request event: %#v", events[0])
	}
	if events[1].Event != "model.provider.response" || events[1].Fields["status"] != "completed" || events[1].Fields["response_chars"] != float64(2) {
		t.Fatalf("unexpected response event: %#v", events[1])
	}
	detail, err := logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	mapped, ok := detail.(map[string]any)
	if !ok || mapped["schema_name"] != "SeedReview" || mapped["caller_role"] != "nerd-verifier" || mapped["prompt"] != nil {
		t.Fatalf("unexpected request detail: %#v", detail)
	}
}

func TestCodexRunnerRetriesAfterAttemptTimeout(t *testing.T) {
	client := &timeoutThenOKClient{}
	logger := gamelog.NewWriter(t.TempDir(), "story-1")
	runner := CodexRunner{
		Client:        client,
		Model:         "gpt-5.5",
		Effort:        "medium",
		Timeout:       10 * time.Millisecond,
		Logger:        logger,
		ToolCategory:  "model",
		CallerRole:    "nerd-verifier",
		CallerSpeaker: "Nerd Verifier",
	}

	out, err := runner.Run(context.Background(), "verify seed")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("output = %q", out)
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d", client.calls)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("events = %#v", events)
	}
	if events[1].Event != "model.provider.failed" || events[1].Fields["will_retry"] != true {
		t.Fatalf("expected timeout failure with retry: %#v", events[1])
	}
	if events[2].Event != "model.provider.request" || events[3].Event != "model.provider.response" {
		t.Fatalf("expected resent request and response: %#v", events)
	}
}

func TestRetryableCodexErrorIncludesSocketDisconnects(t *testing.T) {
	for _, message := range []string{
		"read tcp 192.168.1.39:64645->172.64.155.209:443: read: can't assign requested address",
		"write tcp 10.0.0.2:1234->1.2.3.4:443: write: broken pipe",
		"read tcp 10.0.0.2:1234->1.2.3.4:443: read: connection reset by peer",
		"dial tcp: lookup api.openai.com: temporary failure in name resolution",
		"dial tcp: connect: network is unreachable",
		"context deadline exceeded",
	} {
		if !IsRetryableCodexError(errors.New(message)) {
			t.Fatalf("expected retryable error for %q", message)
		}
	}
}

type fakeLocalToolClient struct {
	body map[string]any
	opts codexoauth.LocalToolOptions
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func assertStrictObjectSchema(t *testing.T, path string, schema any) {
	t.Helper()
	mapped, ok := schema.(map[string]any)
	if !ok {
		return
	}
	switch mapped["type"] {
	case "object":
		if mapped["additionalProperties"] != false {
			t.Fatalf("%s: object schema must set additionalProperties false: %#v", path, mapped["additionalProperties"])
		}
		props, _ := mapped["properties"].(map[string]any)
		required, _ := mapped["required"].([]string)
		for prop := range props {
			if !containsString(required, prop) {
				t.Fatalf("%s: strict object property %q must be required: %#v", path, prop, required)
			}
		}
		for _, field := range required {
			if props[field] == nil {
				t.Fatalf("%s: required field %q missing from properties: %#v", path, field, props)
			}
		}
		for prop, child := range props {
			assertStrictObjectSchema(t, path+"."+prop, child)
		}
	case "array":
		assertStrictObjectSchema(t, path+"[]", mapped["items"])
	default:
		for key, child := range mapped {
			if key == "properties" || key == "required" {
				continue
			}
			assertStrictObjectSchema(t, path+"."+key, child)
		}
	}
}

func (c *fakeLocalToolClient) Create(context.Context, map[string]any) (map[string]any, error) {
	return nil, errors.New("plain create should not be called")
}

func (c *fakeLocalToolClient) CreateWithLocalTools(_ context.Context, body map[string]any, opts codexoauth.LocalToolOptions) (map[string]any, error) {
	c.body = body
	c.opts = opts
	return map[string]any{"output_text": "ok"}, nil
}

type retryClient struct {
	calls int
	errs  []error
}

func (c *retryClient) Create(context.Context, map[string]any) (map[string]any, error) {
	c.calls++
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		return nil, err
	}
	return map[string]any{"output_text": "ok"}, nil
}

type timeoutThenOKClient struct {
	calls int
}

func (c *timeoutThenOKClient) Create(ctx context.Context, _ map[string]any) (map[string]any, error) {
	c.calls++
	if c.calls == 1 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return map[string]any{"output_text": "ok"}, nil
}
