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
	if !strings.Contains(text, "CREATIVE INFLUENCE REFERENCES") || !strings.Contains(text, "source-law") {
		t.Fatalf("missing source lore influence section:\n%s", text)
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
	if !strings.Contains(ctx.RenderText(), "CREATIVE INFLUENCE REFERENCES") {
		t.Fatalf("lore should render as influence references:\n%s", ctx.RenderText())
	}
}

func TestRenderedContextHonorsHardCap(t *testing.T) {
	assembler := testAssembler(staticCards{"tonto": testCard("tonto")})
	assembler.Lore = fakeRetriever{snippets: []lore.Snippet{{
		ID:     "long",
		Text:   strings.Repeat("lore ", 500),
		Source: "long.json",
		Score:  1,
		Kind:   "wiki_lore",
	}}}
	assembler.Options.NarratorSoftCap = 900
	ctx, err := assembler.AssembleNarrator(context.Background(), BuildRequest{Packet: testPacket()})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(ctx.RenderText()); got > 900 {
		t.Fatalf("rendered context exceeded cap: %d", got)
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
		ResumeSummary:       "Rin begins.",
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
