package seed

import (
	"context"
	"encoding/json"
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

func jsonForTest(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}
