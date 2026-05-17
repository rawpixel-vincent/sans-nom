package lore

import (
	"context"
	"errors"
	"testing"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/telemetry"
)

type fakeBridgeSearchClient struct {
	req  bridgeclient.LoreSearchRequest
	resp bridgeclient.LoreSearchResponse
	err  error
}

func (c *fakeBridgeSearchClient) LoreSearch(
	_ context.Context,
	req bridgeclient.LoreSearchRequest,
) (bridgeclient.LoreSearchResponse, error) {
	c.req = req
	return c.resp, c.err
}

type captureSink struct {
	events []telemetry.Event
}

func (s *captureSink) Emit(_ context.Context, event telemetry.Event) {
	s.events = append(s.events, event)
}

func TestBridgeRetrieverSearchMapsSnippets(t *testing.T) {
	volume := 13
	client := &fakeBridgeSearchClient{resp: bridgeclient.LoreSearchResponse{Snippets: []bridgeclient.LoreSnippet{{
		DocID:       "peer_card:lothar",
		Text:        "Lothar is Steelhead.",
		SourcePath:  "lore/packs/metabarons/comics/peer-cards/cards/accepted/lothar.json",
		Score:       0.9,
		Kind:        "peer_card",
		PeerID:      "lothar",
		CharacterID: "steelhead",
		Volume:      &volume,
		Payload:     map[string]any{"doc_id": "peer_card:lothar"},
	}}}}
	sink := &captureSink{}

	results, err := BridgeRetriever{
		Client:     client,
		Telemetry:  sink,
		Collection: "sans_nom_story_test",
		PeerID:     []string{"lothar"},
	}.Search(context.Background(), "Lothar is Steelhead", 5)
	if err != nil {
		t.Fatal(err)
	}
	if client.req.Collection != "sans_nom_story_test" || client.req.Query != "Lothar is Steelhead" || client.req.K != 5 || client.req.PeerID[0] != "lothar" {
		t.Fatalf("unexpected request: %#v", client.req)
	}
	if len(results) != 1 || results[0].ID != "peer_card:lothar" || results[0].Source == "" {
		t.Fatalf("unexpected results: %#v", results)
	}
	if results[0].PeerID != "lothar" || results[0].Volume == nil || *results[0].Volume != 13 {
		t.Fatalf("missing mapped metadata: %#v", results[0])
	}
}

func TestBridgeRetrieverEmitsTelemetryWithoutRawQuery(t *testing.T) {
	client := &fakeBridgeSearchClient{resp: bridgeclient.LoreSearchResponse{}}
	sink := &captureSink{}

	_, err := BridgeRetriever{Client: client, Telemetry: sink}.Search(
		context.Background(),
		"Lothar is Steelhead",
		5,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("expected one event, got %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Name != "lore.search" {
		t.Fatalf("unexpected event name %q", event.Name)
	}
	if event.Fields["query_sha256"] == "" || event.Fields["query_sha256"] == "Lothar is Steelhead" {
		t.Fatalf("query hash leaked or missing: %#v", event.Fields)
	}
	if _, ok := event.Fields["query"]; ok {
		t.Fatalf("raw query should not be emitted: %#v", event.Fields)
	}
	if event.Fields["miss"] != true || event.Fields["hit_count"] != 0 {
		t.Fatalf("unexpected miss fields: %#v", event.Fields)
	}
}

func TestBridgeRetrieverEmitsErrorTelemetry(t *testing.T) {
	client := &fakeBridgeSearchClient{err: errors.New("qdrant unreachable")}
	sink := &captureSink{}

	_, err := BridgeRetriever{Client: client, Telemetry: sink}.Search(context.Background(), "Tonto", 5)

	if err == nil {
		t.Fatal("expected error")
	}
	if sink.events[0].Fields["error"] != "qdrant unreachable" {
		t.Fatalf("missing error telemetry: %#v", sink.events[0].Fields)
	}
}
