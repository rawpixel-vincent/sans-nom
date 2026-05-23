package main

import (
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/app"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

func TestPlayConfigFromSessionPreservesLauncherRouting(t *testing.T) {
	workspace := story.Workspace{StoryID: "story-test", Root: t.TempDir()}
	state := story.StoryState{
		StoryID:              "story-test",
		CurrentNodeID:        "entry",
		GroundLorePackID:     "metabarons",
		GroundLoreCollection: "lorepack_metabarons",
		StoryCollection:      "story_story_test",
	}
	bible := story.StoryBible{StoryID: "story-test", Title: "Test"}
	packet := scene.StoryPacket{ID: "packet-test", EntryNodeID: "entry"}
	session := storyapp.StorySession{
		Workspace: workspace,
		State:     state,
		Bible:     bible,
		Packet:    packet,
		NodeID:    "entry",
	}

	cfg := playConfigFromSession(session, launcherRuntime{
		backend:              "codex",
		bridgeURL:            "http://127.0.0.1:8791",
		storyRoot:            workspace.Root,
		globalCollection:     "fallback_lore",
		storyCollection:      "fallback_story",
		writerScenaristCalls: 7,
		scenaristProvider:    "ZAI",
		scenaristModel:       "glm-5.1",
		scenaristTools:       true,
		refreshWrite:         true,
		commit:               true,
		tts:                  app.PlayTTSConfig{Enabled: true, Voice: "voice"},
	})

	if cfg.Packet.ID != packet.ID || cfg.NodeID != "entry" || cfg.Backend != "codex" {
		t.Fatalf("play config lost route identity: %#v", cfg)
	}
	if !cfg.UseTUI || !cfg.RefreshWrite || !cfg.Commit {
		t.Fatalf("launcher play config should enable TUI/refresh/commit: %#v", cfg)
	}
	if cfg.GroundLorePackID != "metabarons" || cfg.GlobalCollection != "lorepack_metabarons" || cfg.StoryCollection != "fallback_story" {
		t.Fatalf("launcher play config did not prefer story state collections: %#v", cfg)
	}
	if cfg.WriterScenaristCalls != 7 || cfg.ScenaristProvider != "ZAI" || cfg.ScenaristModel != "glm-5.1" || !cfg.ScenaristTools {
		t.Fatalf("launcher model/tool settings not preserved: %#v", cfg)
	}
	if !cfg.TTS.Enabled || cfg.TTS.Voice != "voice" {
		t.Fatalf("launcher TTS settings not preserved: %#v", cfg.TTS)
	}
}
