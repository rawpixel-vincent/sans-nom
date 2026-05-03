package bridge

import (
	"context"
	"encoding/json"
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
