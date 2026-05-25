package seed

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestDefaultPacketValidatesFaithfulSeed(t *testing.T) {
	if err := ValidateFaithfulSeed(DefaultPacket()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFaithfulSeedAllowsLorepackTerms(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[0].Beats[0].Text = "Tonto waits inside the Metabunker."

	if err := ValidateFaithfulSeed(packet); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFaithfulSeedRejectsActorNeedsUntilCardsExist(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[0].ActorNeeds = []scene.ActorNeed{
		{ActorID: "shift-matron", Mode: scene.ActorModePresent},
	}

	if err := ValidateFaithfulSeed(packet); err == nil {
		t.Fatal("expected actor_need rejection")
	}
}

func TestValidateFaithfulSeedRejectsMissingChoiceTargets(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[0].Choices[1].TargetNodeID = "missing-node"

	err := ValidateFaithfulSeed(packet)
	if err == nil || !strings.Contains(err.Error(), "targets missing node") {
		t.Fatalf("expected missing target rejection, got %v", err)
	}
}

func TestValidateFaithfulSeedRequiresPlayableChoicePoint(t *testing.T) {
	packet := DefaultPacket()
	for i := range packet.Nodes {
		packet.Nodes[i].Choices = nil
	}

	err := ValidateFaithfulSeed(packet)
	if err == nil || !strings.Contains(err.Error(), "playable choice point") {
		t.Fatalf("expected playable choice point rejection, got %v", err)
	}
}

func TestValidateFaithfulSeedRequiresTerminalHandoffNotes(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[1].RevisionNotes = nil

	err := ValidateFaithfulSeed(packet)
	if err == nil || !strings.Contains(err.Error(), "terminal seed node") {
		t.Fatalf("expected terminal handoff rejection, got %v", err)
	}
}

func TestContextTextIsCreativeOnly(t *testing.T) {
	text := ContextText()
	for _, required := range []string{
		"scene-first staging",
		"lorepack scenario notes are source-fidelity/background support",
		"does not invent a replacement premise",
		"Opening aim: player, desire, place, pressure",
		"Reader plausibility",
		"daily practice, emotional credibility, material consequence, and follow-through",
		"not from generic house style",
		"first trouble and branch momentum",
		"story_arc and phase fields are optional guidance",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("seed context missing %q:\n%s", required, text)
		}
	}
	for _, forbidden := range []string{
		"creative DNA",
		"new characters, places",
		"ask_scenarist",
		"workshop",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("seed context kept legacy premise-invention wording %q:\n%s", forbidden, text)
		}
	}
}

func TestSeedWriterPromptForbidsPlaceholderContinuationTargets(t *testing.T) {
	text := prompt.SeedWriterPrompt(ContextText())
	for _, required := range []string{
		"Every choice target_node_id must point to a scene node included in this same packet",
		"Terminal handoff nodes have no choices and include revision_notes",
		"Do not use placeholder targets",
		"lorepack scenario notes",
		"story_context",
		"compact playable StoryPacket",
		"at least one real playable choice point",
		"Terminal handoff nodes",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("seed writer prompt missing %q:\n%s", required, text)
		}
	}
}

func TestGenerateUsesVerifierRejection(t *testing.T) {
	rawBundle, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	service := Service{
		Writer:        prompt.StaticRunner{Response: rawBundle},
		Verifier:      prompt.StaticRunner{Response: `{"accepted":false,"blocking_issues":["too polished"],"revision_instructions":"make it structural"}`},
		MaxIterations: 1,
	}

	_, err = service.Generate(context.Background(), ContextText())
	if err == nil {
		t.Fatal("expected verifier rejection")
	}
}

func TestGenerateRetryPatchesPreviousWriterOutput(t *testing.T) {
	invalid := DefaultBundle()
	invalid.Packet.Nodes[1].Choices = []scene.Choice{{
		ID:           "choice_missing",
		Label:        "Bad continuation.",
		TargetNodeID: "node_missing_handoff",
		PlayerIntent: "break validation",
	}}
	rawInvalid, err := jsonForTest(invalid)
	if err != nil {
		t.Fatal(err)
	}
	rawValid, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	runner := &sequenceRunner{responses: []string{rawInvalid, rawValid}}
	service := Service{
		Writer:        runner,
		MaxIterations: 2,
	}

	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 2 {
		t.Fatalf("prompt count = %d", len(runner.prompts))
	}
	retry := runner.prompts[1]
	for _, required := range []string{
		"Continue from this attempt and revise it; do not start a new story seed from scratch",
		"This is a repair pass after validation/review feedback",
		"Do not redo the full lore orientation pass from scratch",
		"Use staged lorepack reads or ask_lore_expert only for targeted checks",
		"node_missing_handoff",
		"Previous raw Writer output to patch",
		"choice_missing",
	} {
		if !strings.Contains(retry, required) {
			t.Fatalf("retry prompt missing %q:\n%s", required, retry)
		}
	}
	if strings.Contains(retry, "Before drafting any StoryBible or StoryPacket, do a lore orientation pass") {
		t.Fatalf("repair retry kept full-orientation wording:\n%s", retry)
	}
}

func TestGenerateRetriesEmptyBibleMetadata(t *testing.T) {
	invalid := DefaultBundle()
	invalid.Bible.Metadata = map[string]string{}
	rawInvalid, err := jsonForTest(invalid)
	if err != nil {
		t.Fatal(err)
	}
	rawValid, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	runner := &sequenceRunner{responses: []string{rawInvalid, rawValid}}
	service := Service{
		Writer:        runner,
		MaxIterations: 2,
	}

	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 2 {
		t.Fatalf("prompt count = %d", len(runner.prompts))
	}
	retry := runner.prompts[1]
	for _, required := range []string{
		"Do not redo the full lore orientation pass from scratch",
		"bible.metadata",
		"seed_version",
		"current_checkpoint",
		"Previous raw Writer output to patch",
	} {
		if !strings.Contains(retry, required) {
			t.Fatalf("retry prompt missing %q:\n%s", required, retry)
		}
	}
}

func TestGenerateLogsSeedWriterNotScenarist(t *testing.T) {
	rawBundle, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	logger := gamelog.NewGlobalWriter(t.TempDir())
	service := Service{
		Writer: prompt.StaticRunner{Response: rawBundle},
		Logger: logger,
	}
	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var sawWriter bool
	for _, event := range events {
		if strings.Contains(event.Event, "scenarist") {
			t.Fatalf("seed producer logged as scenarist: %#v", event)
		}
		if event.Event == "seed.writer.request" {
			sawWriter = true
		}
	}
	if !sawWriter {
		t.Fatalf("missing seed.writer.request event: %#v", events)
	}
}

func TestGenerateLogsWriterAndVerifierRunFailures(t *testing.T) {
	rawBundle, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name      string
		service   func(*gamelog.Writer) Service
		wantEvent string
	}{
		{
			name: "writer",
			service: func(logger *gamelog.Writer) Service {
				return Service{
					Writer: errorRunner{err: errors.New("writer socket dropped")},
					Logger: logger,
				}
			},
			wantEvent: "seed.writer.failed",
		},
		{
			name: "verifier",
			service: func(logger *gamelog.Writer) Service {
				return Service{
					Writer:   prompt.StaticRunner{Response: rawBundle},
					Verifier: errorRunner{err: errors.New("verifier socket dropped")},
					Logger:   logger,
				}
			},
			wantEvent: "seed.verifier.failed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := gamelog.NewGlobalWriter(t.TempDir())
			service := tt.service(logger)
			if _, err := service.Generate(context.Background(), ContextText()); err == nil {
				t.Fatal("expected generation error")
			}
			events, err := gamelog.Read(logger.Path, gamelog.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if event.Event == tt.wantEvent {
					if event.Level != gamelog.LevelError || event.DetailPath == "" {
						t.Fatalf("failure event missing level/detail: %#v", event)
					}
					return
				}
			}
			t.Fatalf("missing %s in events: %#v", tt.wantEvent, events)
		})
	}
}

func TestGenerateRetriesVerifierAfterRecoverableRunError(t *testing.T) {
	rawBundle, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	verifier := &sequenceRunner{
		errs:      []error{errors.New("verifier disconnected")},
		responses: []string{`{"accepted":true}`},
	}
	service := Service{
		Writer:   prompt.StaticRunner{Response: rawBundle},
		Verifier: verifier,
		RecoverRunError: func(stage string, err error) bool {
			return stage == "seed verifier" && strings.Contains(err.Error(), "disconnected")
		},
		MaxIterations: 1,
	}

	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(verifier.prompts) != 2 {
		t.Fatalf("verifier calls = %d, want 2", len(verifier.prompts))
	}
}

func TestGenerateSkipsRetiredSeedWorkshop(t *testing.T) {
	rawBundle, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	runner := &sequenceRunner{responses: []string{rawBundle}}
	logger := gamelog.NewGlobalWriter(t.TempDir())
	service := Service{
		Writer:      runner,
		Logger:      logger,
		WorkshopDir: "retired-workshop",
	}
	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 1 {
		t.Fatalf("prompt count = %d", len(runner.prompts))
	}
	for _, forbidden := range []string{"seed workshop", "Invent three opening situations", "Bell-Hunger", "Seed Workshop Manifest"} {
		if strings.Contains(runner.prompts[0], forbidden) {
			t.Fatalf("final prompt contains retired workshop wording %q:\n%s", forbidden, runner.prompts[0])
		}
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var sawWorkshop bool
	for _, event := range events {
		if strings.HasPrefix(event.Event, "seed.workshop.") {
			sawWorkshop = true
		}
	}
	if sawWorkshop {
		t.Fatalf("retired workshop emitted events: %#v", events)
	}
}

func TestGenerateRetryDoesNotRerunWorkshop(t *testing.T) {
	invalid := DefaultBundle()
	invalid.Packet.Nodes[1].Choices = []scene.Choice{{
		ID:           "choice_missing",
		Label:        "Bad continuation.",
		TargetNodeID: "node_missing_handoff",
		PlayerIntent: "break validation",
	}}
	rawInvalid, err := jsonForTest(invalid)
	if err != nil {
		t.Fatal(err)
	}
	rawValid, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	runner := &sequenceRunner{responses: []string{
		rawInvalid,
		rawValid,
	}}
	service := Service{
		Writer:        runner,
		MaxIterations: 2,
		WorkshopDir:   "retired-workshop",
	}
	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 2 {
		t.Fatalf("prompt count = %d", len(runner.prompts))
	}
	if strings.Contains(runner.prompts[1], "seed staging workshop") {
		t.Fatalf("retry reran workshop instead of final bundle revision:\n%s", runner.prompts[1])
	}
	if !strings.Contains(runner.prompts[1], "Previous raw Writer output to patch") {
		t.Fatalf("retry prompt missing previous raw output:\n%s", runner.prompts[1])
	}
}

func jsonForTest(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

type sequenceRunner struct {
	responses []string
	prompts   []string
	errs      []error
}

func (r *sequenceRunner) Run(_ context.Context, prompt string) (string, error) {
	r.prompts = append(r.prompts, prompt)
	if len(r.errs) > 0 {
		err := r.errs[0]
		r.errs = r.errs[1:]
		return "", err
	}
	index := len(r.prompts) - 1
	if index >= len(r.responses) {
		index = len(r.responses) - 1
	}
	return r.responses[index], nil
}

type errorRunner struct {
	err error
}

func (r errorRunner) Run(context.Context, string) (string, error) {
	return "", r.err
}
