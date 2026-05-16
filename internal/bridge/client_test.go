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
