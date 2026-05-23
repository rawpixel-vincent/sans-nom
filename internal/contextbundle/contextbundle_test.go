package contextbundle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/modelcap"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func TestNarratorContextExcludesUnusedBranchesAndWriterNotes(t *testing.T) {
	ctx, err := testAssembler(staticCards{"tonto": testCard("tonto")}).AssembleNarrator(context.Background(), BuildRequest{
		Packet: testPacket(),
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	for _, forbidden := range []string{"UNUSED_SECRET", "DO_NOT_SHOW", "revision must show", "invalidation must show"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("narrator context leaked %q:\n%s", forbidden, text)
		}
	}
	if strings.Contains(text, "writer_notes") || strings.Contains(text, "revision_notes") || strings.Contains(text, "invalidation_notes") {
		t.Fatalf("narrator context leaked planning fields:\n%s", text)
	}
}

func TestActorContextIsSceneLocal(t *testing.T) {
	ctx, err := testAssembler(staticCards{"tonto": testCard("tonto")}).AssembleNarrator(context.Background(), BuildRequest{
		Packet: testPacket(),
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderActorText()
	if strings.Contains(text, "RECENT WITNESSED CANON") || strings.Contains(text, "Latest prose.") {
		t.Fatalf("actor context leaked prior story canon:\n%s", text)
	}
	if !strings.Contains(text, "CURRENT SCENE STATE") || !strings.Contains(text, "SELECTED NODE") {
		t.Fatalf("actor context missing scene-local material:\n%s", text)
	}
}

func TestWriterContextIncludesFullPacketAndRevisionNotes(t *testing.T) {
	ctx, err := testAssembler(staticCards{"tonto": testCard("tonto")}).AssembleWriter(context.Background(), BuildRequest{
		Packet: testPacket(),
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	for _, required := range []string{"UNUSED_SECRET", "DO_NOT_SHOW", "revision must show", "invalidation must show"} {
		if !strings.Contains(text, required) {
			t.Fatalf("writer context missed %q:\n%s", required, text)
		}
	}
	if !strings.Contains(text, "StoryPacket closure rule") || !strings.Contains(text, "future packet handoffs must be terminal nodes with revision_notes") {
		t.Fatalf("writer context missed packet closure rule:\n%s", text)
	}
	if !strings.Contains(text, "STORY PROGRESSION CONTRACT") ||
		!strings.Contains(text, "Start after the selected node's witnessed outcome") {
		t.Fatalf("writer context missed progression contract:\n%s", text)
	}
	if !strings.Contains(text, "STORY PHASE GUIDANCE") ||
		!strings.Contains(text, "required_next_movement") {
		t.Fatalf("writer context missed phase guidance:\n%s", text)
	}
	if !strings.Contains(text, "Let binding doctrine sharpen desire, opposition, or cost inside the player character's lived order") ||
		!strings.Contains(text, "visible cost, and consequence") {
		t.Fatalf("writer context missed binding doctrine/plausibility rule:\n%s", text)
	}
}

func TestWriterContextReadsWriterOwnedSpentMotifs(t *testing.T) {
	packet := testPacket()
	packet.Metadata = map[string]string{
		"spent_motifs": "mask testimony; witness/testimony proof; servant/household memory",
	}
	bible := testBible()
	bible.CurrentArcPhase = "servant schism"
	bible.NextRequiredRevelation = "the weapon schedule was altered"
	bible.NextRequiredOpposition = "a rival claimant uses the altered memory"
	bible.SpentMotifs = []string{"mask testimony"}
	bible.OpenThreads = []string{"Sava's breath-mask debt"}
	ctx, err := testAssembler(staticCards{"tonto": testCard("tonto")}).AssembleWriter(context.Background(), BuildRequest{
		Packet:   packet,
		NodeID:   "active",
		Decision: scene.PlayerDecision{ChoiceID: "choice_carry_the_mask"},
		Bible:    &bible,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	for _, required := range []string{
		"spent_motifs",
		"mask testimony",
		"witness/testimony proof",
		"servant/household memory",
		"Next required revelation: the weapon schedule was altered",
		"Next required opposition: a rival claimant uses the altered memory",
		"Keep open thread active: Sava's breath-mask debt",
		"Servant-memory proof appears spent",
		"Do not center another servant ledger, mask, copy, or testimony-validity debate",
		"Shift pressure toward weapon/resource claim",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("writer context missing spent-motif guidance %q:\n%s", required, text)
		}
	}
}

func TestWriterContextDoesNotInferSpentMotifsFromProseKeywords(t *testing.T) {
	packet := testPacket()
	packet.ContextSnapshot.LatestNarratorProse = "Saryn carried the witness mask through the service hatch while servants copied its testimony into a ledger."
	ctx, err := testAssembler(staticCards{"tonto": testCard("tonto")}).AssembleWriter(context.Background(), BuildRequest{
		Packet: packet,
		NodeID: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	for _, forbidden := range []string{
		"witness/testimony proof",
		"servant/household memory",
		"Servant-memory proof appears spent",
		"Do not center another servant ledger",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("writer context inferred spent motif %q from prose keywords:\n%s", forbidden, text)
		}
	}
}

func TestWriterContextNarrowsOpenThreadsAfterAuthoredSpentMotifs(t *testing.T) {
	packet := testPacket()
	packet.Metadata = map[string]string{
		"current_arc_phase": "record fracture",
		"next_arc_phase":    "metasurgery investigation",
		"spent_motifs":      "ash record established; QA-9 destroyed; service-band split",
	}
	bible := testBible()
	bible.OpenThreads = []string{
		"Who harvested Iri's wake-command?",
		"Who heard the exterior relay?",
		"What remains inside the missing surgical minutes?",
	}
	ctx, err := testAssembler(staticCards{"tonto": testCard("tonto")}).AssembleWriter(context.Background(), BuildRequest{
		Packet: packet,
		NodeID: "active",
		Bible:  &bible,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	for _, required := range []string{
		"Narrowing phase: close, disprove, merge, or make costly at least one existing open thread before opening a new one.",
		"Do not introduce a new investigation shadow unless it directly resolves or transforms one of the listed open threads.",
		"Open thread available for closure: Who harvested Iri's wake-command?",
		"Open thread available for closure: Who heard the exterior relay?",
		"Open thread available for closure: What remains inside the missing surgical minutes?",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("writer context missing narrowing guidance %q:\n%s", required, text)
		}
	}
	if strings.Contains(text, "Keep open thread active: Who harvested Iri's wake-command?") {
		t.Fatalf("writer context should narrow instead of keeping every open thread active:\n%s", text)
	}
}

func TestWriterBudgetKeepsProgressionBeforePacketReference(t *testing.T) {
	packet := testPacket()
	packet.ContextSnapshot.LatestNarratorProse = "Latest canon: the first challenge was accepted and must now happen."
	packet.Nodes[0].Beats = []scene.Beat{{Text: strings.Repeat("oversized active beat ", 400)}}
	packet.Nodes[1].Beats = []scene.Beat{{Text: strings.Repeat("oversized unused beat ", 400)}}
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	ctx, err := assembler.AssembleWriter(context.Background(), BuildRequest{Packet: packet, NodeID: "active"})
	if err != nil {
		t.Fatal(err)
	}
	text, report := ctx.RenderBudgetedText(modelcap.Budget{InputBudgetTokens: 2500})
	for _, required := range []string{
		"SELECTED NODE",
		"Latest canon: the first challenge was accepted and must now happen.",
		"STORY PROGRESSION CONTRACT",
		"PLANNING CONSTRAINTS",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("budgeted writer context missing %q:\n%s", required, text)
		}
	}
	if !sectionWasCompacted(report, "packet_reference") {
		t.Fatalf("packet reference should compact before required progression sections: %#v", report.Sections)
	}
}

func TestContextIncludesStoryBibleDoctrineSeparateFromInfluenceLore(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "source-law",
		Text:   "Source law pattern, not story canon.",
		Source: "lore/packs/metabarons/index/documents.jsonl",
		Score:  1,
		Kind:   "wiki_lore",
	}}}
	bible := testBible()
	ctx, err := assembler.AssembleWriter(context.Background(), BuildRequest{Packet: testPacket(), Bible: &bible})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	if !strings.Contains(text, "STORY BIBLE DOCTRINE") || !strings.Contains(text, "Law of Refusal") {
		t.Fatalf("missing story bible doctrine:\n%s", text)
	}
	if !strings.Contains(text, "LORE FIDELITY REFERENCES") || !strings.Contains(text, "source-law") {
		t.Fatalf("missing source lore fidelity section:\n%s", text)
	}
}

func TestRuntimeContextsIncludeScenarioContract(t *testing.T) {
	contract := "Lorepack Scenario Contract: Test Pack\n- must_honor:\n  - family pressure matters\n- choice_principles:\n  - choices reveal inheritance"
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	packet := testPacket()
	bible := testBible()
	narratorCtx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{
		Packet:               packet,
		Bible:                &bible,
		ScenarioContractText: contract,
	})
	if err != nil {
		t.Fatal(err)
	}
	writerCtx, err := assembler.AssembleWriter(context.Background(), BuildRequest{
		Packet:               packet,
		Bible:                &bible,
		ScenarioContractText: contract,
	})
	if err != nil {
		t.Fatal(err)
	}
	for label, text := range map[string]string{
		"narrator": narratorCtx.RenderText(),
		"writer":   writerCtx.RenderText(),
	} {
		if !strings.Contains(text, "LOREPACK SCENARIO CONTRACT") || !strings.Contains(text, "family pressure matters") {
			t.Fatalf("%s context missing scenario contract:\n%s", label, text)
		}
	}
	if !strings.Contains(writerCtx.RenderText(), "choice_principles") || !strings.Contains(writerCtx.RenderText(), "must_honor") {
		t.Fatalf("writer context should carry fidelity terms:\n%s", writerCtx.RenderText())
	}
}

func TestWriterContextUsesCompactActorManifest(t *testing.T) {
	cards := staticCards{"tonto": testCard("tonto")}
	for _, id := range []string{"aghora", "lothar", "steelhead__late", "unrelated-one", "unrelated-two"} {
		cards[id] = testCard(id)
	}
	assembler := testAssembler(cards)
	assembler.Options.MaxActorManifestItems = 4
	ctx, err := assembler.AssembleWriter(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.ActorManifest) > 4 {
		t.Fatalf("manifest was not capped: %#v", ctx.ActorManifest)
	}
	text := ctx.RenderText()
	if strings.Contains(text, "unrelated-one") || strings.Contains(text, "unrelated-two") {
		t.Fatalf("writer context included full inventory:\n%s", text)
	}
	if !strings.Contains(text, "tonto") {
		t.Fatalf("writer context missed explicit actor:\n%s", text)
	}
}

func TestWriterBudgetKeepsActorManifestAheadOfLongLore(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "big-lore",
		Text:   strings.Repeat("lore pressure ", 1000),
		Source: "fixture.md",
		Score:  0.9,
		Kind:   "world",
	}}}

	ctx, err := assembler.AssembleWriter(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	text, report := ctx.RenderBudgetedText(modelcap.Budget{InputBudgetTokens: 1200})
	manifestIndex := strings.Index(text, "STORY ACTOR MANIFEST")
	loreIndex := strings.Index(text, "LORE FIDELITY REFERENCES")
	if manifestIndex < 0 || !strings.Contains(text, "tonto") {
		t.Fatalf("writer context missed actor manifest before cap:\n%s", text)
	}
	if loreIndex >= 0 && loreIndex < manifestIndex {
		t.Fatalf("actor manifest should precede lore references:\n%s", text)
	}
	if !sectionWasCompacted(report, "lore_references") {
		t.Fatalf("lore references should compact before actor manifest: %#v", report.Sections)
	}
}

func TestDefaultRuntimeContextDoesNotTruncate(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "long-lore",
		Text:   strings.Repeat("lore pressure ", 1000),
		Source: "long.json",
		Score:  0.9,
		Kind:   "world",
	}}}
	bible := testBible()
	bible.WorldBasics = append(bible.WorldBasics, strings.Repeat("world pressure ", 1000))
	ctx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{
		Packet:               testPacket(),
		Bible:                &bible,
		ScenarioContractText: strings.Repeat("contract pressure ", 1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	text := ctx.RenderText()
	if strings.Contains(text, "[context truncated at render cap]") {
		t.Fatalf("default narrator context should not truncate:\n%s", text)
	}
	for _, required := range []string{"CURRENT SCENE STATE", "SELECTED NODE", "STORY BIBLE DOCTRINE", "LORE FIDELITY REFERENCES", "long-lore"} {
		if !strings.Contains(text, required) {
			t.Fatalf("default narrator context missing %q:\n%s", required, text)
		}
	}
}

func TestNarratorBudgetKeepsSceneMaterialAheadOfOptionalSections(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "big-lore",
		Text:   strings.Repeat("lore pressure ", 1000),
		Source: "fixture.md",
		Score:  0.9,
		Kind:   "world",
	}}}
	bible := testBible()
	bible.WorldBasics = append(bible.WorldBasics, strings.Repeat("world pressure ", 1000))
	ctx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{
		Packet:               testPacket(),
		Bible:                &bible,
		ScenarioContractText: strings.Repeat("contract pressure ", 1000),
	})
	if err != nil {
		t.Fatal(err)
	}
	text, report := ctx.RenderBudgetedText(modelcap.Budget{InputBudgetTokens: 1800})
	for _, required := range []string{"CURRENT SCENE STATE", "PLAYER DECISION", "SELECTED NODE", "RECENT WITNESSED CANON", "Latest prose."} {
		if !strings.Contains(text, required) {
			t.Fatalf("budgeted narrator context lost scene-critical section %q:\n%s", required, text)
		}
	}
	if !sectionWasCompacted(report, "lore_references") && !sectionWasCompacted(report, "story_bible_doctrine") && !sectionWasCompacted(report, "scenario_contract") {
		t.Fatalf("optional lore/bible/contract sections should compact first: %#v", report.Sections)
	}
}

func TestWriterContextDoesNotPromoteLorePeersToActors(t *testing.T) {
	assembler := testAssembler(staticCards{
		"tonto":  testCard("tonto"),
		"lothar": testCard("lothar"),
	})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "peer_card:lothar",
		Text:   "Name: Lothar",
		Source: "lothar.json",
		Score:  0.9,
		Kind:   "peer_card",
		PeerID: "lothar",
	}}}
	ctx, err := assembler.AssembleWriter(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	if got := manifestIDs(ctx.ActorManifest); !reflect.DeepEqual(got, []string{"tonto"}) {
		t.Fatalf("lore peer became an actor candidate: %#v", got)
	}
	if !strings.Contains(ctx.RenderText(), "LORE FIDELITY REFERENCES") {
		t.Fatalf("lore should render as fidelity references:\n%s", ctx.RenderText())
	}
}

func TestAssemblerRequestsSceneScopedHonchoSession(t *testing.T) {
	bridge := &capturingBridge{}
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Bridge = bridge
	packet := testPacket()

	ctx, err := assembler.AssembleWriter(context.Background(), BuildRequest{Packet: packet, NodeID: "active"})
	if err != nil {
		t.Fatal(err)
	}
	expected := scene.SceneSessionID(packet.ContextSnapshot.SessionID, packet.ID, "active")
	if bridge.req.SessionID != expected {
		t.Fatalf("session id = %q, want scene-scoped %q", bridge.req.SessionID, expected)
	}
	if !strings.Contains(strings.Join(ctx.RecentScenes, "\n"), packet.ContextSnapshot.LatestNarratorProse) {
		t.Fatalf("writer context should retain latest local prose when scene session is sparse: %#v", ctx.RecentScenes)
	}
}

func TestBudgetedContextHonorsTokenBudget(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "long",
		Text:   strings.Repeat("lore ", 500),
		Source: "long.json",
		Score:  1,
		Kind:   "wiki_lore",
	}}}
	ctx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	text, report := ctx.RenderBudgetedText(modelcap.Budget{InputBudgetTokens: 650})
	if got := modelcap.EstimateTokens(text); got > 650 {
		t.Fatalf("budgeted context exceeded token budget: %d\nreport: %#v\n%s", got, report, text)
	}
	for _, required := range []string{"CURRENT SCENE STATE", "SELECTED NODE", "MEMORY AND WITNESS RULES"} {
		if !strings.Contains(text, required) {
			t.Fatalf("budgeted context lost required section %q:\n%s", required, text)
		}
	}
}

func TestNoAllCastFanoutFromPresentActorsOrCardInventory(t *testing.T) {
	packet := testPacket()
	packet.ContextSnapshot.PresentActors = []string{"sans-nom", "tonto", "aghora", "lothar", "steelhead__late"}
	ctx, err := testAssembler(staticCards{
		"tonto":           testCard("tonto"),
		"aghora":          testCard("aghora"),
		"lothar":          testCard("lothar"),
		"steelhead__late": testCard("steelhead__late"),
	}).AssembleNarrator(context.Background(), BuildRequest{Packet: packet})
	if err != nil {
		t.Fatal(err)
	}
	if got := actorIDs(ctx.ActorContexts); !reflect.DeepEqual(got, []string{"tonto"}) {
		t.Fatalf("unexpected actor fanout: %#v", got)
	}
}

func TestMissingExplicitActorCardFails(t *testing.T) {
	_, err := testAssembler(staticCards{}).AssembleNarrator(context.Background(), BuildRequest{Packet: testPacket()})
	if err == nil {
		t.Fatal("expected missing actor card error")
	}
	if !strings.Contains(err.Error(), "tonto") {
		t.Fatalf("error should name missing actor: %v", err)
	}
}

func TestMissingExplicitActorCardCanBeSkipped(t *testing.T) {
	assembler := testAssembler(staticCards{})
	assembler.Options.SkipMissingActorCards = true
	ctx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.ActorContexts) != 0 {
		t.Fatalf("actor contexts = %#v", ctx.ActorContexts)
	}
	if !containsSubstring(ctx.Warnings, `actor "tonto" skipped`) {
		t.Fatalf("missing skip warning: %#v", ctx.Warnings)
	}
}

func TestActorRepresentationsKeepPlayerAndCapTargets(t *testing.T) {
	packet := testPacket()
	packet.Nodes[0].ActorNeeds[0].Targets = []string{"steelhead__late", "aghora", "lothar"}
	bridge := fakeBridge{
		cards: map[string][]string{"tonto": {"Name: Tonto"}},
		reps: map[string]string{
			"tonto:sans-nom":        "player rep",
			"tonto:aghora":          "aghora rep",
			"tonto:lothar":          "lothar rep",
			"tonto:steelhead__late": "steelhead rep",
		},
	}
	ctx, err := (Assembler{
		Bridge: bridge,
		Cards:  staticCards{"tonto": testCard("tonto")},
		Options: Options{
			MaxActorTargets: 2,
		},
	}).AssembleNarrator(context.Background(), BuildRequest{Packet: packet})
	if err != nil {
		t.Fatal(err)
	}
	got := representationKeys(ctx.ActorContexts[0].Representations)
	want := []string{"tonto:aghora", "tonto:lothar", "tonto:sans-nom"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected representations: got %#v want %#v", got, want)
	}
}

func TestActorCallSeedsHonchoCardFromLocalFallback(t *testing.T) {
	packet := testPacket()
	bridge := fakeBridge{
		cards: map[string][]string{},
		reps:  map[string]string{"tonto:sans-nom": "player rep"},
	}
	actorCtx, text, warnings, err := (Assembler{
		Bridge: bridge,
		Cards:  staticCards{"tonto": testCard("tonto")},
	}).AssembleActorCall(context.Background(), ActorCallBuildRequest{
		Packet: packet,
		Node:   packet.Nodes[0],
		Need: scene.ActorNeed{
			ActorID: "tonto",
			Mode:    scene.ActorModePresent,
			Reason:  "voice check",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := bridge.cards["tonto"]; len(got) != 1 || got[0] != "Name: tonto" {
		t.Fatalf("local actor card was not seeded to Honcho fallback: %#v", got)
	}
	if !strings.Contains(actorCtx.CardSource, "honcho:tonto seeded from test:tonto") {
		t.Fatalf("unexpected card source: %q", actorCtx.CardSource)
	}
	if !strings.Contains(text, "Name: tonto") {
		t.Fatalf("actor context text missing seeded card:\n%s", text)
	}
	if !containsSubstring(warnings, "fallback card source for tonto") {
		t.Fatalf("expected fallback warning, got %#v", warnings)
	}
}

func TestActorCallCanDisableHonchoCardSeeding(t *testing.T) {
	packet := testPacket()
	bridge := fakeBridge{
		cards: map[string][]string{},
		reps:  map[string]string{"tonto:sans-nom": "player rep"},
	}
	actorCtx, _, warnings, err := (Assembler{
		Bridge: bridge,
		Cards:  staticCards{"tonto": testCard("tonto")},
		Options: Options{
			DisableCardSeeding: true,
		},
	}).AssembleActorCall(context.Background(), ActorCallBuildRequest{
		Packet: packet,
		Node:   packet.Nodes[0],
		Need: scene.ActorNeed{
			ActorID: "tonto",
			Mode:    scene.ActorModePresent,
			Reason:  "voice check",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := bridge.cards["tonto"]; len(got) != 0 {
		t.Fatalf("actor card should not be seeded in read-only mode: %#v", got)
	}
	if actorCtx.CardSource != "test:tonto" {
		t.Fatalf("expected local card source, got %q", actorCtx.CardSource)
	}
	if !containsSubstring(warnings, "fallback card source for tonto") {
		t.Fatalf("expected fallback warning, got %#v", warnings)
	}
}

func TestLoreHitsDeduplicateSortAndTruncate(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "low",
		Text:   "short",
		Source: "low.json",
		Score:  0.2,
		Kind:   "wiki_lore",
	}, {
		ID:     "high",
		Text:   "this text is deliberately long",
		Source: "high.json",
		Score:  0.9,
		Kind:   "peer_card",
		PeerID: "tonto",
	}, {
		ID:     "high",
		Text:   "duplicate",
		Source: "duplicate.json",
		Score:  0.1,
		Kind:   "peer_card",
	}}}
	assembler.Options.MaxLoreChars = 12
	ctx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	if got := loreIDs(ctx.LoreHits); !reflect.DeepEqual(got, []string{"high", "low"}) {
		t.Fatalf("unexpected lore order/dedupe: %#v", got)
	}
	if !strings.HasSuffix(ctx.LoreHits[0].Text, "...") {
		t.Fatalf("expected truncated lore hit: %#v", ctx.LoreHits[0])
	}
	if !hasWarning(ctx.Warnings, "lore hit high truncated") {
		t.Fatalf("missing truncation warning: %#v", ctx.Warnings)
	}
}

func TestLocalCardStoreLoadsAcceptedPeerCardsBeforeMarkdown(t *testing.T) {
	dir := t.TempDir()
	actorDir := filepath.Join(dir, "actors")
	acceptedDir := filepath.Join(dir, "accepted")
	nonPeerDir := filepath.Join(dir, "non-peer")
	for _, path := range []string{actorDir, acceptedDir, nonPeerDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(actorDir, "tonto.md"), "Name: Markdown Tonto\n")
	writeFile(t, filepath.Join(actorDir, "aghora.md"), "Name: Aghora\n")
	writePeerCard(t, filepath.Join(acceptedDir, "tonto.json"), "tonto", []string{"Name: Accepted Tonto"})
	writePeerCard(t, filepath.Join(nonPeerDir, "non-peer.json"), "non-peer", []string{"Name: Non Peer"})

	store, err := LoadLocalCardStore(actorDir, acceptedDir)
	if err != nil {
		t.Fatal(err)
	}
	tonto, ok := store.LocalCard("tonto")
	if !ok {
		t.Fatal("missing tonto card")
	}
	if !strings.Contains(tonto.Text, "Accepted Tonto") || strings.Contains(tonto.Text, "Markdown Tonto") {
		t.Fatalf("accepted card should override markdown: %#v", tonto)
	}
	if _, ok := store.LocalCard("non-peer"); ok {
		t.Fatal("non-peer card should not be loaded from the accepted card directory")
	}
	if _, ok := store.LocalCard("aghora"); !ok {
		t.Fatal("markdown fallback card should be loaded")
	}
}

func TestStoryCardStoreOverlaysStoryActorCards(t *testing.T) {
	dir := t.TempDir()
	acceptedDir := filepath.Join(dir, "accepted")
	if err := os.MkdirAll(acceptedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePeerCard(t, filepath.Join(acceptedDir, "iri.json"), "Iri Itta", []string{"Name: Accepted Iri", "TRAIT: Old source"})
	base, err := LoadLocalCardStore("", acceptedDir)
	if err != nil {
		t.Fatal(err)
	}
	storyDir := filepath.Join(dir, "story")
	actorDir := filepath.Join(storyDir, "cards", "actors")
	if err := os.MkdirAll(actorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(actorDir, "Iri Itta.md"), "Name: Iri Itta\nTRAIT: Story-local agency\n")

	store, err := LoadStoryCardStore(base, storyDir)
	if err != nil {
		t.Fatal(err)
	}
	card, ok := store.LocalCard("Iri Itta")
	if !ok {
		t.Fatal("missing story-local Iri card")
	}
	if !strings.Contains(card.Text, "Story-local agency") || strings.Contains(card.Text, "Old source") {
		t.Fatalf("story card should override base card: %#v", card)
	}
	if _, ok := base.LocalCard("Iri Itta"); !ok {
		t.Fatal("base store should remain intact")
	}
}

func testAssembler(cards staticCards) Assembler {
	return Assembler{Cards: cards}
}

func testPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "packet-1",
		EntryNodeID: "active",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:         "phase0-dev",
			SessionID:           "session-1",
			Location:            "Eastern hall",
			Time:                "childhood",
			LatestNarratorProse: "Latest prose.",
			PresentActors:       []string{"sans-nom", "tonto"},
			Representations: map[string]string{
				"tonto:sans-nom": "snapshot rep",
			},
		},
		Nodes: []scene.SceneNode{{
			ID:    "active",
			Title: "Active Node",
			Beats: []scene.Beat{{
				ID:   "beat-1",
				Text: "The active beat.",
			}},
			Choices: []scene.Choice{{
				ID:           "go-unused",
				Label:        "Go unused",
				TargetNodeID: "unused",
			}},
			ActorNeeds: []scene.ActorNeed{{
				ActorID: "tonto",
				Mode:    scene.ActorModePresent,
				Reason:  "Tonto watches.",
			}},
			MemoryIntents: []scene.MemoryIntent{{
				ID:        "canon",
				Summary:   "The active beat happened.",
				Policy:    scene.MemoryPolicyCanon,
				Witnesses: []string{"sans-nom", "tonto"},
			}},
			RevisionNotes:     []string{"node revision hidden"},
			InvalidationNotes: []string{"node invalidation hidden"},
			WriterNotes:       "DO_NOT_SHOW",
		}, {
			ID:      "unused",
			Title:   "Unused Node",
			Beats:   []scene.Beat{{Text: "UNUSED_SECRET"}},
			Choices: []scene.Choice{},
		}},
		RevisionNotes:     []string{"revision must show"},
		InvalidationNotes: []string{"invalidation must show"},
	}
}

type staticCards map[string]Card

func (s staticCards) LocalCard(actorID string) (Card, bool) {
	card, ok := s[actorID]
	return card, ok
}

func (s staticCards) ActorIDs() []string {
	ids := make([]string, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func testCard(actorID string) Card {
	return Card{
		ActorID: actorID,
		Text:    "Name: " + actorID,
		Source:  "test:" + actorID,
	}
}

func testBible() story.StoryBible {
	return story.StoryBible{
		StoryID: "phase0-dev",
		Title:   "Test Story",
		Premise: "A test premise.",
		PlayerCharacter: story.PlayerCharacter{
			PeerID:      "sans-nom",
			Name:        "Rin",
			Description: "A test player.",
			Desire:      "To choose.",
			Wound:       "Recorded as a function.",
			Boundary:    "Will not disappear into function.",
		},
		PlayerWorldConnection: "Rin's body is recognized by the city.",
		WorldBasics:           []string{"World one", "World two", "World three"},
		OriginalLaws: []story.OriginalLaw{{
			ID:             "refusal",
			Name:           "Law of Refusal",
			Description:    "The world records what a person refuses to become.",
			SourcePattern:  "source structure only",
			Enforcement:    "Public witnesses remember refusals as binding debt.",
			WhyProfound:    "It protects identity from function.",
			NotMerelyCruel: "It can shield the powerless from erased consent.",
		}},
		FirstTurnExposition: []string{"world", "player", "connection", "law", "pressure"},
		StoryArc:            []string{"opening pressure", "changed status", "wider opposition", "public consequence"},
		ResumeSummary:       "Rin begins.",
		Metadata: map[string]string{
			"seed_version":        "test-v1",
			"generated_at":        "test",
			"resume_point":        "entry",
			"current_checkpoint":  "opening pressure",
			"continuation_hooks":  "continue the test pressure",
			"canon_validated":     "true",
			"open_threads":        "test thread",
			"gameplay_mode":       "test",
			"ground_lore_pack_id": "metabarons",
		},
	}
}

type fakeRetriever struct {
	snippets []lore.Snippet
	err      error
}

func (r fakeRetriever) Search(context.Context, string, int) ([]lore.Snippet, error) {
	return r.snippets, r.err
}

type fakeBridge struct {
	cards map[string][]string
	reps  map[string]string
	err   error
}

func (b fakeBridge) SceneContext(_ context.Context, req bridgeclient.SceneContextRequest) (bridgeclient.SceneContextResponse, error) {
	if b.err != nil {
		return bridgeclient.SceneContextResponse{}, b.err
	}
	return bridgeclient.SceneContextResponse{
		WorkspaceID:     req.WorkspaceID,
		SessionID:       req.SessionID,
		RecentScenes:    []string{"bridge scene"},
		Representations: map[string]string{"narrator:sans-nom": "narrator rep"},
	}, nil
}

func (b fakeBridge) Representation(_ context.Context, req bridgeclient.RepresentationRequest) (bridgeclient.RepresentationResponse, error) {
	if b.err != nil {
		return bridgeclient.RepresentationResponse{}, b.err
	}
	return bridgeclient.RepresentationResponse{
		Observer:       req.Observer,
		Observed:       req.Observed,
		Representation: b.reps[req.Observer+":"+req.Observed],
	}, nil
}

func (b fakeBridge) GetCard(_ context.Context, _ string, peerID string, _ string) ([]string, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.cards[peerID], nil
}

func (b fakeBridge) PatchCard(_ context.Context, _ string, peerID string, patch bridgeclient.PeerCardPatch) ([]string, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.cards != nil {
		b.cards[peerID] = append([]string(nil), patch.Facts...)
	}
	return append([]string(nil), patch.Facts...), nil
}

type capturingBridge struct {
	req bridgeclient.SceneContextRequest
}

func (b *capturingBridge) SceneContext(_ context.Context, req bridgeclient.SceneContextRequest) (bridgeclient.SceneContextResponse, error) {
	b.req = req
	return bridgeclient.SceneContextResponse{
		WorkspaceID:     req.WorkspaceID,
		SessionID:       req.SessionID,
		RecentScenes:    []string{},
		Representations: map[string]string{},
	}, nil
}

func (b *capturingBridge) Representation(_ context.Context, req bridgeclient.RepresentationRequest) (bridgeclient.RepresentationResponse, error) {
	return bridgeclient.RepresentationResponse{Observer: req.Observer, Observed: req.Observed}, nil
}

func (b *capturingBridge) GetCard(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}

func (b *capturingBridge) PatchCard(context.Context, string, string, bridgeclient.PeerCardPatch) ([]string, error) {
	return nil, nil
}

func actorIDs(actors []ActorContext) []string {
	ids := make([]string, 0, len(actors))
	for _, actor := range actors {
		ids = append(ids, actor.ActorID)
	}
	sort.Strings(ids)
	return ids
}

func manifestIDs(actors []ActorManifestItem) []string {
	ids := make([]string, 0, len(actors))
	for _, actor := range actors {
		ids = append(ids, actor.PeerID)
	}
	sort.Strings(ids)
	return ids
}

func representationKeys(reps []Representation) []string {
	keys := make([]string, 0, len(reps))
	for _, rep := range reps {
		keys = append(keys, rep.Observer+":"+rep.Observed)
	}
	sort.Strings(keys)
	return keys
}

func loreIDs(hits []LoreHit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	return ids
}

func hasWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func sectionWasCompacted(report RenderBudgetReport, id string) bool {
	for _, section := range report.Sections {
		if section.ID == id {
			return section.Status == "trimmed" || section.Status == "dropped"
		}
	}
	return false
}

func writeFile(t *testing.T, path string, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePeerCard(t *testing.T, path string, peerID string, facts []string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"peer_card_id": peerID,
		"card":         facts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
