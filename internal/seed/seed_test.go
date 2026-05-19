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

func TestValidateOriginalSeedRequiresRunwayIntroChoices(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[0].Choices[1].TargetNodeID = "turn_3_submit_hand"

	err := ValidateOriginalSeed(packet)
	if err == nil || !strings.Contains(err.Error(), "turn_1_intro choices must all target the same") {
		t.Fatalf("expected cosmetic intro target rejection, got %v", err)
	}
}

func TestValidateOriginalSeedRequiresThreeRealDecisionChoices(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[1].Choices = packet.Nodes[1].Choices[:2]

	err := ValidateOriginalSeed(packet)
	if err == nil || !strings.Contains(err.Error(), "exactly 3 real choices") {
		t.Fatalf("expected decision choice count rejection, got %v", err)
	}
}

func TestValidateOriginalSeedRequiresTerminalThirdTurnBranches(t *testing.T) {
	packet := DefaultPacket()
	packet.Nodes[2].Choices = []scene.Choice{{
		ID:           "bad_followup",
		Label:        "Bad followup.",
		TargetNodeID: "turn_3_steal_pearl",
	}}

	err := ValidateOriginalSeed(packet)
	if err == nil || !strings.Contains(err.Error(), "must be terminal with no choices") {
		t.Fatalf("expected terminal branch rejection, got %v", err)
	}
}

func TestContextTextIsCreativeOnly(t *testing.T) {
	text := ContextText()
	for _, required := range []string{
		"Opening aim: a practical situation already in motion",
		"Reader plausibility",
		"daily practice, emotional credibility, visible cost, and consequence",
		"Material imagination",
		"infrastructure, ecology, transport, weather",
		"supports conflict rather than replacing story with procedure",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("seed context missing %q:\n%s", required, text)
		}
	}
	for _, forbidden := range []string{
		"StoryPacket",
		"StoryBible",
		"three-turn runway",
		"turn_1_intro",
		"target_node_id",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("seed context leaked final bundle contract %q:\n%s", forbidden, text)
		}
	}
}

func TestSeedWriterPromptForbidsPlaceholderContinuationTargets(t *testing.T) {
	text := prompt.SeedWriterPrompt(ContextText())
	for _, required := range []string{
		"Every choice target_node_id must point to a scene node included in this same packet",
		"Each turn_3 branch node has no choices and has revision_notes",
		"Do not use placeholder targets",
		"original binding doctrine",
		"support element for character conflict",
		"three-turn runway",
		"turn_1_intro",
		"turn_2_decision",
		"exactly 3 meaningful choices",
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
		"## Germs\n### Bell-Hunger\nA bell image.",
		"## Elements\nCharacter, POV, Setting, Plot, Theme, Style/Tone.",
		"## Engine\nConflict and opposition.",
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
	for _, required := range []string{"Output Markdown only", "Stay free-form", "no JSON, schema field names, node ids, final bundle, or player-facing prose"} {
		if !strings.Contains(runner.prompts[0], required) {
			t.Fatalf("workshop prompt missing %q:\n%s", required, runner.prompts[0])
		}
	}
	for _, required := range []string{
		"Invent three opening situations for a playable speculative adventure story",
		"Return three numbered sections: 1, 2, and 3",
		"one practical job going wrong",
		"Use plain language and low lore density",
		"at most one invented place name and one invented tool or device name",
		"material causes",
		"End each situation with the first player choice",
	} {
		if !strings.Contains(runner.prompts[0], required) {
			t.Fatalf("imagination workshop prompt missing %q:\n%s", required, runner.prompts[0])
		}
	}
	for _, forbidden := range []string{"StoryBible", "StoryPacket", "target_node_id", "The seed packet is a three-turn runway", "turn_1_intro", "Every choice target must be an included node"} {
		if strings.Contains(runner.prompts[0], forbidden) {
			t.Fatalf("workshop prompt leaked final contract %q:\n%s", forbidden, runner.prompts[0])
		}
	}
	if !strings.Contains(runner.prompts[2], "Dramatic Engine Turn") ||
		!strings.Contains(runner.prompts[2], "cast friction") ||
		!strings.Contains(runner.prompts[2], "local doctrine only where it sharpens conflict") {
		t.Fatalf("engine workshop prompt has wrong story framing:\n%s", runner.prompts[2])
	}
	final := runner.prompts[4]
	if !strings.Contains(final, "Seed Workshop Manifest") || strings.Contains(final, "## Germs\n### Bell-Hunger") {
		t.Fatalf("final prompt should include manifest but not raw workshop text:\n%s", final)
	}
	if strings.Contains(final, "Bell-Hunger") || strings.Contains(strings.ToLower(final), "bell image") {
		t.Fatalf("final prompt should not propagate raw workshop motifs through summaries:\n%s", final)
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
