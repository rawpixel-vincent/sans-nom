package admin

import (
	"context"
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
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

func TestAdminListsStoriesAndReadsDetail(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	if err := gamelog.NewWriter(ws.Dir, state.StoryID).Append(gamelog.Event{Category: "story", Event: "story.created"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
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
	if detail.Summary.GroundLorePackID != "metabarons" || detail.GroundLorePack == nil {
		t.Fatalf("missing ground lore pack in detail: %#v", detail.Summary)
	}
}

func TestAdminListsLorePacks(t *testing.T) {
	server := httptest.NewServer(NewServer(t.TempDir(), bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Packs []map[string]any `json:"packs"`
	}
	getJSON(t, server.URL+"/admin/api/lore-packs", &resp)
	if len(resp.Packs) == 0 || resp.Packs[0]["id"] != "metabarons" {
		t.Fatalf("unexpected lore packs: %#v", resp)
	}
}

func TestAdminExposesGlobalLogsAndDetails(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{
		Category: "story",
		Event:    "story.seed.started",
		Detail:   map[string]any{"prompt": "seed request"},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var logs struct {
		Events []gamelog.Event `json:"events"`
	}
	getJSON(t, server.URL+"/admin/api/logs", &logs)
	if len(logs.Events) != 1 || logs.Events[0].DetailPath == "" {
		t.Fatalf("unexpected global logs: %#v", logs.Events)
	}
	var detail struct {
		Detail map[string]any `json:"detail"`
	}
	getJSON(t, server.URL+"/admin/api/logs/detail?path="+logs.Events[0].DetailPath, &detail)
	if detail.Detail["prompt"] != "seed request" {
		t.Fatalf("unexpected global detail: %#v", detail.Detail)
	}
}

func TestAdminDeletesGlobalLog(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{Category: "story", Event: "delete.me", Detail: map[string]any{"payload": "gone"}}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{Category: "story", Event: "delete.me.too"}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{Category: "story", Event: "keep.me"}); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(global.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/api/logs?event_id="+events[0].EventID+"&event_id="+events[1].EventID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	remaining, err := gamelog.Read(global.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Event != "keep.me" {
		t.Fatalf("unexpected remaining global logs: %#v", remaining)
	}
}

func TestAdminDeletesStoryLog(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{Category: "story", Event: "delete.me"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{Category: "story", Event: "keep.me"}); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/api/stories/"+state.StoryID+"/logs?event_id="+events[0].EventID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	remaining, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Event != "keep.me" {
		t.Fatalf("unexpected remaining story logs: %#v", remaining)
	}
}

func TestAdminCreateStoryUsesSelectedPack(t *testing.T) {
	root := t.TempDir()
	var gotPackID string
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{
		NewStory: func(_ context.Context, packID string) (storyapp.StorySession, error) {
			gotPackID = packID
			ws, state, err := story.InitWorkspace(story.InitOptions{
				StoryID:              "created-story",
				Root:                 root,
				GroundLorePackID:     packID,
				GroundLoreCollection: story.DefaultGroundLoreCollection,
				Replace:              true,
			}, seed.DefaultBundle())
			if err != nil {
				return storyapp.StorySession{}, err
			}
			return storyapp.ResumeStory(context.Background(), state.StoryID, storyapp.StoryActionOptions{StoryRoot: ws.Root})
		},
	}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories", "application/json", strings.NewReader(`{"ground_lore_pack_id":"metabarons"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	if gotPackID != "metabarons" {
		t.Fatalf("pack id = %q", gotPackID)
	}
}

func TestAdminRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
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
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
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
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
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
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
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
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient(bridgeServer.URL, ""), "", Callbacks{}))
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
		GroundLorePackID:     "metabarons",
		GroundLoreCollection: story.DefaultGroundLoreCollection,
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
