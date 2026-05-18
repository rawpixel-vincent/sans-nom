package seed

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestDefaultPacketValidatesOriginalSeed(t *testing.T) {
	if err := ValidateOriginalSeed(DefaultPacket()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPacketDoesNotUseCanonProperNouns(t *testing.T) {
	text := strings.ToLower(createdStoryText(DefaultPacket()))
	for _, term := range forbiddenCanonTerms() {
		if strings.Contains(text, term) {
			t.Fatalf("default seed contains source-canon term %q:\n%s", term, text)
		}
	}
}

func TestValidateOriginalSeedRejectsLiteralCanon(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[0].Beats[0].Text = "Tonto waits inside the Metabunker."

	if err := ValidateOriginalSeed(packet); err == nil {
		t.Fatal("expected literal canon rejection")
	}
}

func TestValidateOriginalSeedAllowsCanonInRevisionConstraint(t *testing.T) {
	packet := DefaultPacket()
	packet.RevisionNotes = []string{"No Metabarons canon names."}

	if err := ValidateOriginalSeed(packet); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOriginalSeedRejectsActorNeedsUntilCardsExist(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[0].ActorNeeds = []scene.ActorNeed{
		{ActorID: "shift-matron", Mode: scene.ActorModePresent},
	}

	if err := ValidateOriginalSeed(packet); err == nil {
		t.Fatal("expected actor_need rejection")
	}
}

func TestContextTextForbidsPlaceholderContinuationTargets(t *testing.T) {
	text := ContextText()
	for _, required := range []string{
		"Every choice target must be an included node",
		"make that branch node terminal with revision_notes",
		"placeholder continuation choice",
		"original binding doctrine",
		"player character's world",
		"social cause, emotional credibility, visible cost, and consequence",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("seed context missing %q:\n%s", required, text)
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
		"node_missing_handoff",
		"Previous raw Writer output to patch",
		"choice_missing",
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

func TestGenerateRunsSeedWorkshopBeforeFinalBundle(t *testing.T) {
	rawBundle, err := jsonForTest(DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	runner := &sequenceRunner{responses: []string{
		"## Germs\nA law image.",
		"## Elements\nCharacter, POV, Setting, Plot, Theme, Style/Tone.",
		"## Law\nEnforcement and refusal.",
		"## Opening\nChoices as pressure, not ids.",
		rawBundle,
	}}
	logger := gamelog.NewGlobalWriter(t.TempDir())
	workshopDir := filepath.Join(t.TempDir(), "workshop")
	service := Service{
		Writer:      runner,
		Logger:      logger,
		WorkshopDir: workshopDir,
	}
	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 5 {
		t.Fatalf("prompt count = %d", len(runner.prompts))
	}
	for _, required := range []string{"Output Markdown only", "Do not output JSON", "Do not use schema field names or target_node_id values"} {
		if !strings.Contains(runner.prompts[0], required) {
			t.Fatalf("workshop prompt missing %q:\n%s", required, runner.prompts[0])
		}
	}
	if !strings.Contains(runner.prompts[2], "Doctrine And Plausibility Turn") ||
		!strings.Contains(runner.prompts[2], "original binding doctrine") ||
		!strings.Contains(runner.prompts[2], "player character's lived social order") {
		t.Fatalf("doctrine workshop prompt has wrong law framing:\n%s", runner.prompts[2])
	}
	final := runner.prompts[4]
	if !strings.Contains(final, "Seed Workshop Manifest") || strings.Contains(final, "## Germs\nA law image.") {
		t.Fatalf("final prompt should include manifest but not raw workshop text:\n%s", final)
	}
	if !strings.Contains(final, "Repeated motifs in workshop output are not requirements") ||
		!strings.Contains(final, "discard any motif that became too repetitive") {
		t.Fatalf("final prompt should prevent workshop motif propagation:\n%s", final)
	}
	for _, rel := range []string{
		filepath.Join("01-imagination", "prompt.txt"),
		filepath.Join("01-imagination", "output.md"),
		filepath.Join("01-imagination", "metadata.json"),
		"manifest.md",
		"manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(workshopDir, rel)); err != nil {
			t.Fatalf("missing workshop artifact %s: %v", rel, err)
		}
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var sawCompleted bool
	for _, event := range events {
		if event.Event == "seed.workshop.completed" {
			sawCompleted = true
		}
	}
	if !sawCompleted {
		t.Fatalf("missing seed.workshop.completed event: %#v", events)
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
		"germs",
		"elements",
		"law",
		"opening",
		rawInvalid,
		rawValid,
	}}
	service := Service{
		Writer:        runner,
		MaxIterations: 2,
		WorkshopDir:   filepath.Join(t.TempDir(), "workshop"),
	}
	if _, err := service.Generate(context.Background(), ContextText()); err != nil {
		t.Fatal(err)
	}
	if len(runner.prompts) != 6 {
		t.Fatalf("prompt count = %d", len(runner.prompts))
	}
	if strings.HasPrefix(runner.prompts[5], "You are the Creative Writer for the Sans-Nom story seed workshop.") {
		t.Fatalf("retry reran workshop instead of final bundle revision:\n%s", runner.prompts[5])
	}
	if !strings.Contains(runner.prompts[5], "Previous raw Writer output to patch") {
		t.Fatalf("retry prompt missing previous raw output:\n%s", runner.prompts[5])
	}
}

func jsonForTest(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

type sequenceRunner struct {
	responses []string
	prompts   []string
}

func (r *sequenceRunner) Run(_ context.Context, prompt string) (string, error) {
	r.prompts = append(r.prompts, prompt)
	index := len(r.prompts) - 1
	if index >= len(r.responses) {
		index = len(r.responses) - 1
	}
	return r.responses[index], nil
}
