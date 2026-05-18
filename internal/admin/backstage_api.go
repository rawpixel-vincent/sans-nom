package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func (s *Server) handleGlobalBackstage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	messages, err := s.globalBackstageMessages(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"messages": messages})
}

func (s *Server) handleStoryBackstage(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	messages, err := s.storyBackstageMessages(r, storyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"messages": messages})
}

func (s *Server) handleGlobalBackstageStream(w http.ResponseWriter, r *http.Request) {
	s.handleBackstageStream(w, r, "")
}

func (s *Server) handleStoryBackstageStream(w http.ResponseWriter, r *http.Request, storyID string) {
	s.handleBackstageStream(w, r, storyID)
}

func (s *Server) handleBackstageStream(w http.ResponseWriter, r *http.Request, storyID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func() error {
		var (
			messages []backstageMessage
			err      error
		)
		if storyID == "" {
			messages, err = s.globalBackstageMessages(r)
		} else {
			messages, err = s.storyBackstageMessages(r, storyID)
		}
		if err != nil {
			messages = []backstageMessage{{
				ID:         "backstage-stream-error",
				Time:       time.Now().UTC().Format(time.RFC3339Nano),
				Role:       "system",
				Speaker:    "System",
				Kind:       "error",
				Status:     "failed",
				Text:       err.Error(),
				Preview:    err.Error(),
				Visibility: "admin_only",
			}}
		}
		payload, err := json.Marshal(map[string]any{"messages": messages})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: backstage\ndata: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := send(); err != nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := send(); err != nil {
				return
			}
		}
	}
}

func (s *Server) globalBackstageMessages(r *http.Request) ([]backstageMessage, error) {
	tail := backstageTail(r)
	writer := gamelog.NewGlobalWriter(s.StoryRoot)
	events, err := gamelog.Read(writer.Path, gamelog.Filter{Tail: tail})
	if err != nil {
		return nil, err
	}
	return buildBackstageMessages(writer.RootDir, writer, events), nil
}

func (s *Server) storyBackstageMessages(r *http.Request, storyID string) ([]backstageMessage, error) {
	tail := backstageTail(r)
	ws, state, err := story.LoadWorkspace(s.StoryRoot, storyID)
	if err != nil {
		return nil, err
	}
	writer := gamelog.NewWriter(ws.Dir, state.StoryID)
	events, err := gamelog.Read(filepath.Join(ws.LogsDir(), "events.jsonl"), gamelog.Filter{Tail: tail})
	if err != nil {
		return nil, err
	}
	return buildBackstageMessages(ws.Dir, writer, events), nil
}

func backstageTail(r *http.Request) int {
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		return 200
	}
	if tail > 2000 {
		return 2000
	}
	return tail
}
