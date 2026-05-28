package actortool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestAskActorWritesPlanningMaterialArtifact(t *testing.T) {
	registry, err := actors.NewRegistry([]actors.Actor{{
		ID:          "tonto",
		DisplayName: "Tonto",
		Card:        "Name: Tonto",
	}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	logger := gamelog.NewWriter(root, "story-1")
	bridge := &fakeActorBridge{
		cards: map[string][]string{},
		reps:  map[string]string{"tonto:sans-nom": "Tonto sees Sans-Nom as careful."},
	}
	packet := actorToolPacket()
	runner := &ToolRunner{
		Registry: registry,
		Invoker: actors.StaticInvoker{Outputs: map[string]actors.Output{
			"tonto": {
				Interior: "Private unease.",
				Dialogue: "I remember the door.",
				Action:   "Tonto touches the lintel.",
			},
		}},
		Assembler: contextbundle.Assembler{
			Bridge: bridge,
			Cards: testCards{"tonto": {
				ActorID: "tonto",
				Text:    "Name: Tonto",
				Source:  "local:tonto",
			}},
		},
		Packet:        packet,
		NodeID:        "entry",
		OutputDir:     filepath.Join(root, "actors"),
		WorkspaceRoot: root,
		VisiblePaths:  []string{"story", "actors"},
		Logger:        logger,
		CallerRole:    "writer",
		CallerName:    "Writer",
		RunID:         "run-actor",
		Turn:          3,
	}

	rawArgs := `{"actor_id":"tonto","mode":"present","scene_setup":"The old door opens.","question":"What does Tonto risk saying?","targets":[],"node_id":""}`
	got, err := runner.Handle(context.Background(), root, rawArgs)
	if err != nil {
		t.Fatal(err)
	}
	resp := got.(map[string]any)
	material := resp["material"].(actors.ActorMaterial)
	if material.ActorID != "tonto" || material.Dialogue != "I remember the door." {
		t.Fatalf("unexpected material: %#v", material)
	}
	if !strings.Contains(material.UsedCardSource, "honcho:tonto seeded from local:tonto") {
		t.Fatalf("unexpected card source: %q", material.UsedCardSource)
	}
	if len(material.UsedRepresentations) != 1 || !strings.Contains(material.UsedRepresentations[0], "careful") {
		t.Fatalf("missing used representation: %#v", material.UsedRepresentations)
	}
	if material.ArtifactPath != "actors/actor-call-001/material.json" {
		t.Fatalf("artifact path = %q", material.ArtifactPath)
	}
	if got := bridge.cards["tonto"]; len(got) != 1 || got[0] != "Name: Tonto" {
		t.Fatalf("Honcho seed card not patched: %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(root, material.ArtifactPath))
	if err != nil {
		t.Fatal(err)
	}
	var artifact actors.ActorMaterial
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.ArtifactPath != material.ArtifactPath || artifact.Interior != "Private unease." {
		t.Fatalf("unexpected material artifact: %#v", artifact)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "actor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Event != "actor.tool.request" || events[1].Event != "actor.tool.completed" {
		t.Fatalf("unexpected actor tool logs: %#v", events)
	}
	if events[0].RunID != "run-actor" || events[0].Turn != 3 || events[0].Fields["actor_id"] != "tonto" {
		t.Fatalf("actor request log missing run metadata: %#v", events[0])
	}
	requestDetail, err := logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toString(requestDetail), "planning-only") || !strings.Contains(toString(requestDetail), "Tonto sees Sans-Nom") {
		t.Fatalf("actor request detail missing behind-the-scenes context: %#v", requestDetail)
	}
	completedDetail, err := logger.ReadDetail(events[1].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(toString(completedDetail), "actor-call-001") || !strings.Contains(toString(completedDetail), "Private unease") {
		t.Fatalf("actor completed detail missing material/artifact: %#v", completedDetail)
	}
}

func TestAskActorAllowsSceneLocalActor(t *testing.T) {
	registry, err := actors.NewRegistry([]actors.Actor{{ID: "tonto"}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	runner := &ToolRunner{
		Registry:      registry,
		Invoker:       actors.StaticInvoker{Outputs: map[string]actors.Output{"selka_blankwrist": {Dialogue: "I will guide the cut, not kneel to it."}}},
		Assembler:     contextbundle.Assembler{},
		Packet:        actorToolPacket(),
		NodeID:        "entry",
		OutputDir:     filepath.Join(root, "actors"),
		WorkspaceRoot: root,
	}
	got, err := runner.Handle(context.Background(), runner.WorkspaceRoot, `{"actor_id":"selka_blankwrist","mode":"present","scene_setup":"Selka stands in the refuge with blank wrists.","question":"What does Selka refuse?","targets":[],"node_id":""}`)
	if err != nil {
		t.Fatal(err)
	}
	material := got.(map[string]any)["material"].(actors.ActorMaterial)
	if material.ActorID != "selka_blankwrist" || !strings.Contains(material.Dialogue, "guide the cut") {
		t.Fatalf("unexpected scene-local material: %#v", material)
	}
	if material.UsedCardSource != "scene-local:selka_blankwrist" {
		t.Fatalf("card source = %q", material.UsedCardSource)
	}
}

func actorToolPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID: "packet-1",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID: "ws",
			SessionID:   "session",
		},
		Nodes: []scene.SceneNode{{
			ID:    "entry",
			Title: "Entry",
			Beats: []scene.Beat{{Text: "Tonto waits."}},
		}},
	}
}

func toString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

type testCards map[string]contextbundle.Card

func (s testCards) LocalCard(actorID string) (contextbundle.Card, bool) {
	card, ok := s[actorID]
	return card, ok
}

func (s testCards) ActorIDs() []string {
	ids := make([]string, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	return ids
}

type fakeActorBridge struct {
	cards map[string][]string
	reps  map[string]string
}

func (b *fakeActorBridge) SceneContext(_ context.Context, req bridgeclient.SceneContextRequest) (bridgeclient.SceneContextResponse, error) {
	return bridgeclient.SceneContextResponse{
		WorkspaceID:     req.WorkspaceID,
		SessionID:       req.SessionID,
		Representations: map[string]string{},
	}, nil
}

func (b *fakeActorBridge) Representation(_ context.Context, req bridgeclient.RepresentationRequest) (bridgeclient.RepresentationResponse, error) {
	return bridgeclient.RepresentationResponse{
		Observer:       req.Observer,
		Observed:       req.Observed,
		Representation: b.reps[req.Observer+":"+req.Observed],
	}, nil
}

func (b *fakeActorBridge) GetCard(_ context.Context, _ string, peerID string, _ string) ([]string, error) {
	return b.cards[peerID], nil
}

func (b *fakeActorBridge) PatchCard(_ context.Context, _ string, peerID string, patch bridgeclient.PeerCardPatch) ([]string, error) {
	b.cards[peerID] = append([]string(nil), patch.Facts...)
	return append([]string(nil), patch.Facts...), nil
}
