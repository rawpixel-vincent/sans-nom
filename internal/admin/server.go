package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

type RefreshWriterFunc func(ctx context.Context, storyID string) error
type NewStoryFunc func(ctx context.Context, groundLorePackID string) (storyapp.StorySession, error)
type ListActorsFunc func(ctx context.Context, storyID string) ([]ActorSummary, error)
type ActorChatFunc func(ctx context.Context, storyID string, req ActorChatRequest) (ActorChatResponse, error)

type Callbacks struct {
	RefreshWriter RefreshWriterFunc
	NewStory      NewStoryFunc
	ListActors    ListActorsFunc
	ActorChat     ActorChatFunc
}

type ActorSummary struct {
	ActorID     string `json:"actor_id"`
	DisplayName string `json:"display_name"`
	CardSource  string `json:"card_source"`
	CardPreview string `json:"card_preview"`
}

type ActorChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type ActorChatRequest struct {
	ActorID string             `json:"actor_id"`
	Message string             `json:"message"`
	History []ActorChatMessage `json:"history,omitempty"`
}

type ActorChatResponse struct {
	ActorID  string             `json:"actor_id"`
	Interior string             `json:"interior,omitempty"`
	Dialogue string             `json:"dialogue,omitempty"`
	Action   string             `json:"action,omitempty"`
	Warnings []string           `json:"warnings,omitempty"`
	Model    modelmeta.Metadata `json:"model,omitempty"`
}

type Server struct {
	StoryRoot    string
	PackRoot     string
	Bridge       bridgeclient.Client
	FrontendDir  string
	DevServerURL string
	Callbacks    Callbacks
}

type controlRequest struct {
	Confirm             bool   `json:"confirm"`
	Turn                int    `json:"turn,omitempty"`
	Replace             bool   `json:"replace,omitempty"`
	StoryIDConfirmation string `json:"story_id_confirmation,omitempty"`
}

type storyDetail struct {
	Summary        storyapp.StorySummary `json:"summary"`
	State          story.StoryState      `json:"state"`
	Bible          story.StoryBible      `json:"bible"`
	Packet         any                   `json:"packet"`
	Turns          []story.TurnRecord    `json:"turns"`
	TurnCount      int                   `json:"turn_count"`
	Narration      []story.NarrationFile `json:"narration"`
	Paths          map[string]string     `json:"paths"`
	GroundLorePack *lorepack.Pack        `json:"ground_lore_pack,omitempty"`
}

type fileInfo struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
	IsDir    bool   `json:"is_dir"`
	Category string `json:"category,omitempty"`
}

var errBadControlRequest = errors.New("bad control request")

type createStoryRequest struct {
	GroundLorePackID string `json:"ground_lore_pack_id"`
}

func NewServer(storyRoot string, bridge bridgeclient.Client, frontendDir string, callbacks Callbacks) *Server {
	if strings.TrimSpace(storyRoot) == "" {
		storyRoot = story.DefaultRoot
	}
	if strings.TrimSpace(frontendDir) == "" {
		frontendDir = filepath.Join("admin", "dist")
	}
	return &Server{
		StoryRoot:    storyRoot,
		PackRoot:     findPackRoot(),
		Bridge:       bridge,
		FrontendDir:  frontendDir,
		DevServerURL: os.Getenv("SANS_NOM_ADMIN_DEV_SERVER"),
		Callbacks:    callbacks,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/admin/api/") {
		s.serveAPI(w, r)
		return
	}
	s.serveFrontend(w, r)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/api/"), "/")
	if path == "status" {
		s.handleStatus(w, r)
		return
	}
	if path == "lore-packs" {
		s.handleLorePacks(w, r)
		return
	}
	if path == "stories" {
		s.handleStories(w, r)
		return
	}
	if path == "logs" {
		s.handleGlobalLogs(w, r)
		return
	}
	if path == "logs/detail" {
		s.handleGlobalLogDetail(w, r)
		return
	}
	if path == "backstage" {
		s.handleGlobalBackstage(w, r)
		return
	}
	if path == "backstage/stream" {
		s.handleGlobalBackstageStream(w, r)
		return
	}
	if strings.HasPrefix(path, "stories/") {
		s.handleStoryPath(w, r, strings.Split(path, "/")[1:])
		return
	}
	writeError(w, http.StatusNotFound, "unknown admin endpoint")
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	info, err := s.Bridge.HealthInfo(r.Context())
	resp := map[string]any{
		"bridge_url": s.Bridge.BaseURL,
		"ok":         err == nil,
		"error":      "",
		"health":     info,
		"story_root": filepath.Clean(s.StoryRoot),
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

func (s *Server) handleStories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		summaries, err := storyapp.ListStories(s.StoryRoot)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"stories": summaries})
	case http.MethodPost:
		s.handleCreateStory(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleLorePacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	packs, err := lorepack.ListPacks(s.PackRoot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"packs": packs})
}

func (s *Server) handleCreateStory(w http.ResponseWriter, r *http.Request) {
	if s.Callbacks.NewStory == nil {
		writeError(w, http.StatusInternalServerError, "new story is not configured")
		return
	}
	var req createStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session, err := s.Callbacks.NewStory(r.Context(), req.GroundLorePackID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"story_id": session.State.StoryID,
		"summary":  storySummaryFromSession(session),
	})
}

func (s *Server) handleStoryPath(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, http.StatusNotFound, "missing story id")
		return
	}
	storyID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch action {
	case "":
		s.handleStoryDetail(w, r, storyID)
	case "file":
		s.handleStoryFile(w, r, storyID)
	case "logs":
		if len(parts) > 2 && parts[2] == "detail" {
			s.handleStoryLogDetail(w, r, storyID)
			return
		}
		s.handleStoryLogs(w, r, storyID)
	case "backstage":
		if len(parts) > 2 && parts[2] == "stream" {
			s.handleStoryBackstageStream(w, r, storyID)
			return
		}
		s.handleStoryBackstage(w, r, storyID)
	case "actors":
		s.handleStoryActors(w, r, storyID)
	case "actor-chat":
		s.handleActorChat(w, r, storyID)
	case "model-runs":
		s.handleModelRuns(w, r, storyID)
	case "index":
		s.handleStoryIndex(w, r, storyID)
	case "control":
		if len(parts) < 3 {
			writeError(w, http.StatusNotFound, "missing control action")
			return
		}
		s.handleControl(w, r, storyID, parts[2])
	default:
		writeError(w, http.StatusNotFound, "unknown story endpoint")
	}
}

func (s *Server) handleStoryDetail(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	session, err := storyapp.ResumeStory(r.Context(), storyID, storyapp.StoryActionOptions{StoryRoot: s.StoryRoot})
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	turns, _ := session.Workspace.LoadTurns()
	if turns == nil {
		turns = []story.TurnRecord{}
	}
	narration, _ := session.Workspace.LoadNarrationFiles()
	if narration == nil {
		narration = []story.NarrationFile{}
	}
	detail := storyDetail{
		Summary:   storySummaryFromSession(session),
		State:     session.State,
		Bible:     session.Bible,
		Packet:    session.Packet,
		Turns:     turns,
		TurnCount: len(turns),
		Narration: narration,
		Paths: map[string]string{
			"state":     "state.json",
			"bible":     "bible.json",
			"turns":     "turns.jsonl",
			"narration": "narration",
			"index":     "index/documents.jsonl",
			"manifest":  "index/manifest.json",
			"logs":      "logs/events.jsonl",
			"modelRuns": "model-runs",
		},
	}
	if pack, err := lorepack.GetPack(s.PackRoot, session.State.SourceLorePackID()); err == nil {
		detail.GroundLorePack = &pack
	}
	writeJSON(w, detail)
}

func (s *Server) handleStoryActors(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.Callbacks.ListActors == nil {
		writeError(w, http.StatusInternalServerError, "actor listing is not configured")
		return
	}
	if _, _, err := story.LoadWorkspace(s.StoryRoot, storyID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	actors, err := s.Callbacks.ListActors(r.Context(), storyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"actors": actors})
}

func (s *Server) handleActorChat(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.Callbacks.ActorChat == nil {
		writeError(w, http.StatusInternalServerError, "actor chat is not configured")
		return
	}
	if _, _, err := story.LoadWorkspace(s.StoryRoot, storyID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var req ActorChatRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.ActorID) == "" {
		writeError(w, http.StatusBadRequest, "actor_id is required")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	response, err := s.Callbacks.ActorChat(r.Context(), storyID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, response)
}

func (s *Server) handleStoryFile(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ws, _, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	path, err := safeStoryPath(ws.Dir, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType(path))
	_, _ = w.Write(data)
}

func (s *Server) handleStoryLogs(w http.ResponseWriter, r *http.Request, storyID string) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodDelete:
		s.handleDeleteStoryLog(w, r, storyID)
		return
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ws, _, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	events, err := gamelog.Read(filepath.Join(ws.LogsDir(), "events.jsonl"), gamelog.Filter{
		Level:    r.URL.Query().Get("level"),
		Category: r.URL.Query().Get("category"),
		Event:    r.URL.Query().Get("event"),
		Tail:     tail,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"events": events, "categories": gamelog.Categories()})
}

func (s *Server) handleDeleteStoryLog(w http.ResponseWriter, r *http.Request, storyID string) {
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	eventIDs := r.URL.Query()["event_id"]
	deleted, err := gamelog.NewWriter(ws.Dir, state.StoryID).DeleteMany(eventIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(deleted) == 0 {
		writeError(w, http.StatusNotFound, "log event not found")
		return
	}
	writeJSON(w, deleteLogResponse(deleted))
}

func (s *Server) handleStoryLogDetail(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	detail, err := gamelog.NewWriter(ws.Dir, state.StoryID).ReadDetail(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"detail": detail})
}

func (s *Server) handleGlobalLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodDelete:
		s.handleDeleteGlobalLog(w, r)
		return
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	events, err := gamelog.Read(gamelog.GlobalEventsPath(s.StoryRoot), gamelog.Filter{
		Level:    r.URL.Query().Get("level"),
		Category: r.URL.Query().Get("category"),
		Event:    r.URL.Query().Get("event"),
		Tail:     tail,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"events": events, "categories": gamelog.Categories()})
}

func (s *Server) handleDeleteGlobalLog(w http.ResponseWriter, r *http.Request) {
	eventIDs := r.URL.Query()["event_id"]
	deleted, err := gamelog.NewGlobalWriter(s.StoryRoot).DeleteMany(eventIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(deleted) == 0 {
		writeError(w, http.StatusNotFound, "log event not found")
		return
	}
	writeJSON(w, deleteLogResponse(deleted))
}

func (s *Server) handleGlobalLogDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	detail, err := gamelog.NewGlobalWriter(s.StoryRoot).ReadDetail(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"detail": detail})
}

func (s *Server) handleModelRuns(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ws, _, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	files, err := listFiles(ws.Dir, "model-runs")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"files": files})
}

func (s *Server) handleStoryIndex(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	docs, _ := ws.ReadIndexDocuments()
	var manifest any
	_ = readJSON(filepath.Join(ws.IndexDir(), "manifest.json"), &manifest)
	writeJSON(w, map[string]any{
		"collection": state.StoryCollection,
		"manifest":   manifest,
		"documents":  docs,
	})
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request, storyID string, action string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req controlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "control action requires confirm=true")
		return
	}
	var err error
	var result any = map[string]any{"ok": true, "action": action}
	switch action {
	case "rebuild-index":
		err = s.rebuildIndex(r.Context(), storyID)
	case "reingest-index":
		err = s.reingestIndex(r.Context(), storyID, req.Replace)
	case "recommit-turn":
		err = s.recommitTurn(r.Context(), storyID, req.Turn)
	case "refresh-writer":
		err = s.refreshWriter(r.Context(), storyID)
	case "delete-story":
		result, err = s.deleteStory(r.Context(), storyID, req.StoryIDConfirmation)
	default:
		writeError(w, http.StatusNotFound, "unknown control action")
		return
	}
	if err != nil {
		s.logControl(storyID, action+".failed", gamelog.LevelError, err.Error(), map[string]any{"error": err.Error()})
		if errors.Is(err, errBadControlRequest) {
			writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), errBadControlRequest.Error()+": "))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if action != "delete-story" {
		s.logControl(storyID, action+".completed", gamelog.LevelInfo, action+" completed", nil)
	}
	writeJSON(w, result)
}

func (s *Server) rebuildIndex(_ context.Context, storyID string) error {
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		return err
	}
	bible, err := ws.LoadBible()
	if err != nil {
		return err
	}
	packet, err := ws.LoadPacket(state)
	if err != nil {
		return err
	}
	turns, err := ws.LoadTurns()
	if err != nil {
		return err
	}
	return ws.WriteIndex(story.BuildIndexDocuments(bible, packet, turns), state.StoryCollection)
}

func (s *Server) reingestIndex(ctx context.Context, storyID string, replace bool) error {
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		return err
	}
	docs, err := ws.ReadIndexDocuments()
	if err != nil {
		return err
	}
	_, err = s.Bridge.IngestStoryIndex(ctx, bridgeclient.StoryIndexIngestRequest{
		Collection: state.StoryCollection,
		Documents:  bridgeStoryDocs(docs),
		Replace:    replace,
	})
	return err
}

func (s *Server) recommitTurn(ctx context.Context, storyID string, turn int) error {
	if turn <= 0 {
		return errors.New("turn must be greater than zero")
	}
	ws, _, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		return err
	}
	turns, err := ws.LoadTurns()
	if err != nil {
		return err
	}
	for _, record := range turns {
		if record.Turn == turn {
			return s.Bridge.CommitScene(ctx, record.Commit)
		}
	}
	return fmt.Errorf("turn %d not found", turn)
}

func (s *Server) refreshWriter(ctx context.Context, storyID string) error {
	if s.Callbacks.RefreshWriter == nil {
		return errors.New("writer refresh is not configured")
	}
	return s.Callbacks.RefreshWriter(ctx, storyID)
}

func (s *Server) deleteStory(ctx context.Context, storyID string, confirmation string) (map[string]any, error) {
	if confirmation != storyID {
		return nil, fmt.Errorf("%w: story deletion requires story_id_confirmation to match the story id", errBadControlRequest)
	}
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		return nil, err
	}
	s.logControl(storyID, "delete-story.started", gamelog.LevelWarn, "story deletion started", map[string]any{
		"workspace_id": state.WorkspaceID,
		"collection":   state.StoryCollection,
		"local_dir":    filepath.Clean(ws.Dir),
	})
	external, err := s.Bridge.DeleteStory(ctx, bridgeclient.StoryDeleteRequest{
		WorkspaceID: state.WorkspaceID,
		Collection:  state.StoryCollection,
	})
	if err != nil {
		return nil, err
	}
	if err := removeStoryDir(s.StoryRoot, ws.Dir); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":       true,
		"action":   "delete-story",
		"external": external,
		"local": map[string]any{
			"story_id": storyID,
			"deleted":  true,
		},
	}, nil
}

func (s *Server) logControl(storyID string, event string, level string, message string, fields map[string]any) {
	ws, err := story.NewWorkspace(s.StoryRoot, storyID)
	if err != nil {
		return
	}
	_ = gamelog.NewWriter(ws.Dir, storyID).Append(gamelog.Event{
		Level:    level,
		Category: "control",
		Event:    event,
		Message:  message,
		Fields:   fields,
	})
}

func storySummaryFromSession(session storyapp.StorySession) storyapp.StorySummary {
	return storyapp.StorySummary{
		StoryID:              session.State.StoryID,
		Title:                session.Bible.Title,
		Label:                firstNonEmpty(session.Bible.Title, session.State.StoryID),
		LatestTurn:           session.State.LatestTurn,
		UpdatedAt:            session.State.UpdatedAt,
		CreatedAt:            session.State.CreatedAt,
		Path:                 filepath.Clean(session.Workspace.Dir),
		GroundLorePackID:     session.State.SourceLorePackID(),
		GroundLoreCollection: session.State.SourceLoreCollection(),
	}
}

func bridgeStoryDocs(docs []story.IndexDocument) []bridgeclient.StoryIndexDocument {
	out := make([]bridgeclient.StoryIndexDocument, 0, len(docs))
	for _, doc := range docs {
		out = append(out, bridgeclient.StoryIndexDocument{
			DocID:        doc.DocID,
			PointID:      doc.PointID,
			Kind:         doc.Kind,
			SourcePath:   doc.SourcePath,
			SourceSHA256: doc.SourceSHA256,
			TextSHA256:   doc.TextSHA256,
			Text:         doc.Text,
			StoryID:      doc.StoryID,
			PacketID:     doc.PacketID,
			NodeID:       doc.NodeID,
			Turn:         doc.Turn,
			Heading:      doc.Heading,
			Payload:      doc.Payload,
		})
	}
	return out
}

func deleteLogResponse(events []gamelog.Event) map[string]any {
	eventIDs := make([]string, 0, len(events))
	detailDeleted := 0
	for _, event := range events {
		eventIDs = append(eventIDs, event.EventID)
		if event.DetailPath != "" {
			detailDeleted++
		}
	}
	return map[string]any{
		"ok":                   true,
		"deleted_count":        len(events),
		"detail_deleted_count": detailDeleted,
		"event_ids":            eventIDs,
	}
}

func listFiles(root string, rel string) ([]fileInfo, error) {
	base, err := safeStoryPath(root, rel)
	if err != nil {
		return nil, err
	}
	var files []fileInfo
	if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
		return files, nil
	}
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, fileInfo{
			Path:    filepath.ToSlash(relative),
			Name:    d.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
			IsDir:   d.IsDir(),
		})
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func safeStoryPath(root string, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", errors.New("path escapes story root")
	}
	path := filepath.Join(root, clean)
	back, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(back, "..") {
		return "", errors.New("path escapes story root")
	}
	return path, nil
}

func removeStoryDir(root string, storyDir string) error {
	root = filepath.Clean(root)
	storyDir = filepath.Clean(storyDir)
	back, err := filepath.Rel(root, storyDir)
	if err != nil || back == "." || strings.HasPrefix(back, "..") || filepath.IsAbs(back) {
		return errors.New("refusing to remove path outside story root")
	}
	return os.RemoveAll(storyDir)
}

func (s *Server) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if s.DevServerURL != "" {
		target, err := url.Parse(s.DevServerURL)
		if err == nil {
			httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
			return
		}
	}
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/admin")
	if path == "" || path == "/" {
		path = "/index.html"
	}
	full, err := safeStaticPath(s.FrontendDir, strings.TrimPrefix(path, "/"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := os.Stat(full); err != nil {
		full = filepath.Join(s.FrontendDir, "index.html")
	}
	http.ServeFile(w, r, full)
}

func safeStaticPath(root string, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes static root")
	}
	return filepath.Join(root, clean), nil
}

func contentType(path string) string {
	switch filepath.Ext(path) {
	case ".json":
		return "application/json"
	case ".jsonl", ".log", ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func findPackRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "lore", "packs")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "."
		}
		wd = parent
	}
}
