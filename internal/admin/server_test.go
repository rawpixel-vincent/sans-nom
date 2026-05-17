package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/seed"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func TestAdminListsStoriesAndReadsDetail(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	if err := gamelog.NewWriter(ws.Dir, state.StoryID).Append(gamelog.Event{Category: "story", Event: "story.created"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", nil))
	defer server.Close()

	var stories struct {
		Stories []map[string]any `json:"stories"`
	}
	getJSON(t, server.URL+"/admin/api/stories", &stories)
	if len(stories.Stories) != 1 || stories.Stories[0]["story_id"] != state.StoryID {
		t.Fatalf("unexpected stories: %#v", stories)
	}

	var detail storyDetail
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID, &detail)
	if detail.State.StoryID != state.StoryID || detail.Bible.Title == "" || detail.Packet == nil || detail.Turns == nil {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestAdminRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", nil))
	defer server.Close()

	resp, err := http.Get(server.URL + "/admin/api/stories/" + state.StoryID + "/file?path=../state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %s", resp.Status)
	}
}

func TestAdminControlRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", nil))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/rebuild-index", "application/json", strings.NewReader(`{"confirm":false}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected confirmation rejection, got %s", resp.Status)
	}
}

func TestAdminRebuildIndexEmitsControlLog(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", nil))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/rebuild-index", "application/json", strings.NewReader(`{"confirm":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %s", resp.Status)
	}
	events, err := gamelog.Read(ws.LogsDir()+"/events.jsonl", gamelog.Filter{Category: "control"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[len(events)-1].Event != "rebuild-index.completed" {
		t.Fatalf("missing control log: %#v", events)
	}
}

func TestAdminDeleteStoryRequiresTypedConfirmation(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", nil))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/delete-story", "application/json", strings.NewReader(`{"confirm":true,"story_id_confirmation":"wrong"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected typed confirmation rejection, got %s", resp.Status)
	}
	if _, _, err := story.LoadWorkspace(root, state.StoryID); err != nil {
		t.Fatalf("story should still exist: %v", err)
	}
}

func TestAdminDeleteStoryRemovesLocalAndExternal(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	var got bridgeclient.StoryDeleteRequest
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/story/delete" || r.Method != http.MethodPost {
			t.Fatalf("unexpected bridge request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(bridgeclient.StoryDeleteResponse{
			WorkspaceID:             got.WorkspaceID,
			Collection:              got.Collection,
			QdrantCollectionDeleted: true,
			HonchoWorkspaceDeleted:  true,
			HonchoSessionsDeleted:   1,
			HonchoLive:              true,
		})
	}))
	defer bridgeServer.Close()
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient(bridgeServer.URL, ""), "", nil))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/delete-story", "application/json", strings.NewReader(`{"confirm":true,"story_id_confirmation":"admin-story"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	if got.WorkspaceID != state.WorkspaceID || got.Collection != state.StoryCollection {
		t.Fatalf("unexpected bridge delete request: %#v", got)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Fatalf("story dir should be removed, stat err=%v", err)
	}
}

func createAdminStory(t *testing.T, root string) (story.Workspace, story.StoryState) {
	t.Helper()
	ws, state, err := story.InitWorkspace(story.InitOptions{
		StoryID:              "admin-story",
		Root:                 root,
		GlobalLoreCollection: story.DefaultGlobalLoreCollection,
		Replace:              true,
	}, seed.DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	return ws, state
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}
