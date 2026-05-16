package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestCommitSceneRejectsPlanningArtifactBeforeHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	err := client.CommitScene(context.Background(), scene.Commit{
		FinalProse: "prose",
		Events: []scene.CanonEvent{{
			PeerID:    "narrator",
			Content:   "future",
			Source:    scene.CommitSourceWriterDraft,
			Witnesses: []string{"sans-nom"},
		}},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if called {
		t.Fatal("sidecar should not be called for invalid commit")
	}
}

func TestSceneContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/scene/context" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(SceneContextResponse{
			WorkspaceID:     "ws",
			SessionID:       "session",
			RecentScenes:    []string{"one"},
			Representations: map[string]string{"tonto:sans-nom": "curious child"},
		})
	}))
	defer server.Close()

	resp, err := NewClient(server.URL, "token").SceneContext(context.Background(), SceneContextRequest{
		WorkspaceID: "ws",
		SessionID:   "session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Representations["tonto:sans-nom"] == "" {
		t.Fatalf("missing representation: %#v", resp)
	}
}

func TestLoreSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/lore/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var req LoreSearchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req.Query != "Lothar is Steelhead" || req.K != 5 || req.PeerID[0] != "lothar" {
			t.Fatalf("unexpected request: %#v", req)
		}
		_ = json.NewEncoder(w).Encode(LoreSearchResponse{Snippets: []LoreSnippet{{
			DocID:      "peer_card:lothar",
			Text:       "Lothar is Steelhead.",
			SourcePath: "lore/canon/comics/peer-cards/cards/accepted/lothar.json",
			Score:      0.9,
			Kind:       "peer_card",
			PeerID:     "lothar",
		}}})
	}))
	defer server.Close()

	resp, err := NewClient(server.URL, "token").LoreSearch(context.Background(), LoreSearchRequest{
		Query:  "Lothar is Steelhead",
		K:      5,
		PeerID: []string{"lothar"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Snippets) != 1 || resp.Snippets[0].DocID != "peer_card:lothar" {
		t.Fatalf("unexpected snippets: %#v", resp.Snippets)
	}
}

func TestRepresentation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/repr" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req RepresentationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.WorkspaceID != "phase0-dev" || req.Observer != "tonto" || req.Observed != "sans-nom" || req.TopK != 3 {
			t.Fatalf("unexpected request: %#v", req)
		}
		_ = json.NewEncoder(w).Encode(RepresentationResponse{
			Observer:       "tonto",
			Observed:       "sans-nom",
			Representation: "careful child",
		})
	}))
	defer server.Close()

	resp, err := NewClient(server.URL, "").Representation(context.Background(), RepresentationRequest{
		WorkspaceID: "phase0-dev",
		Observer:    "tonto",
		Observed:    "sans-nom",
		TopK:        3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Representation != "careful child" {
		t.Fatalf("unexpected representation: %#v", resp)
	}
}

func TestGetCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/peer/lothar/card" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("workspace_id"); got != "phase0-dev" {
			t.Fatalf("unexpected workspace_id %q", got)
		}
		if got := r.URL.Query().Get("target"); got != "sans-nom" {
			t.Fatalf("unexpected target %q", got)
		}
		_ = json.NewEncoder(w).Encode(PeerCardResponse{
			PeerID: "lothar",
			Target: "sans-nom",
			Facts:  []string{"Name: Lothar"},
		})
	}))
	defer server.Close()

	facts, err := NewClient(server.URL, "").GetCard(context.Background(), "phase0-dev", "lothar", "sans-nom")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0] != "Name: Lothar" {
		t.Fatalf("unexpected facts: %#v", facts)
	}
}
