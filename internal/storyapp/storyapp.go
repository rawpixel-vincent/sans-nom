package storyapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

type GenerateSeedBundleFunc func(context.Context, StoryActionOptions) (story.SeedBundle, error)
type SeedExternalFunc func(context.Context, StorySession, story.SeedBundle) error

type StoryActionOptions struct {
	StoryID              string
	StoryRoot            string
	GlobalLoreCollection string
	StoryCollection      string
	ReplaceStory         bool

	GenerateSeedBundle GenerateSeedBundleFunc
	SeedExternal       SeedExternalFunc
}

type StorySession struct {
	Workspace story.Workspace
	State     story.StoryState
	Bible     story.StoryBible
	Packet    scene.StoryPacket
	NodeID    string
	Bundle    story.SeedBundle
}

type StorySummary struct {
	StoryID    string
	Title      string
	Label      string
	LatestTurn int
	UpdatedAt  string
	CreatedAt  string
	Path       string
}

func (s StorySummary) SecondaryText() string {
	updated := s.UpdatedAt
	if updated == "" {
		updated = "unknown"
	}
	return fmt.Sprintf("%s  turn %d  updated %s", s.StoryID, s.LatestTurn, updated)
}

func NewStory(ctx context.Context, opts StoryActionOptions) (StorySession, error) {
	if opts.GenerateSeedBundle == nil {
		return StorySession{}, errors.New("generate seed bundle callback is required")
	}
	bundle, err := opts.GenerateSeedBundle(ctx, opts)
	if err != nil {
		return StorySession{}, err
	}
	ws, state, err := story.InitWorkspace(story.InitOptions{
		StoryID:              opts.StoryID,
		Root:                 opts.StoryRoot,
		GlobalLoreCollection: firstNonEmpty(opts.GlobalLoreCollection, story.DefaultGlobalLoreCollection),
		StoryCollection:      opts.StoryCollection,
		Replace:              opts.ReplaceStory,
	}, bundle)
	if err != nil {
		return StorySession{}, err
	}
	session, err := sessionFromWorkspace(ws, state)
	if err != nil {
		return StorySession{}, err
	}
	if opts.SeedExternal != nil {
		if err := opts.SeedExternal(ctx, session, session.Bundle); err != nil {
			return StorySession{}, err
		}
	}
	return session, nil
}

func ResumeStory(_ context.Context, storyID string, opts StoryActionOptions) (StorySession, error) {
	ws, state, err := story.LoadWorkspace(opts.StoryRoot, storyID)
	if err != nil {
		return StorySession{}, err
	}
	return sessionFromWorkspace(ws, state)
}

func ListStories(root string) ([]StorySummary, error) {
	if strings.TrimSpace(root) == "" {
		root = story.DefaultRoot
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var summaries []StorySummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ws, state, err := story.LoadWorkspace(root, entry.Name())
		if err != nil {
			continue
		}
		bible, err := ws.LoadBible()
		if err != nil {
			continue
		}
		label := strings.TrimSpace(bible.Title)
		if label == "" {
			label = state.StoryID
		}
		summaries = append(summaries, StorySummary{
			StoryID:    state.StoryID,
			Title:      bible.Title,
			Label:      label,
			LatestTurn: state.LatestTurn,
			UpdatedAt:  state.UpdatedAt,
			CreatedAt:  state.CreatedAt,
			Path:       filepath.Clean(ws.Dir),
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		ti := summaryTime(summaries[i])
		tj := summaryTime(summaries[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return summaries[i].StoryID < summaries[j].StoryID
	})
	return summaries, nil
}

func sessionFromWorkspace(ws story.Workspace, state story.StoryState) (StorySession, error) {
	bible, err := ws.LoadBible()
	if err != nil {
		return StorySession{}, err
	}
	packet, err := ws.LoadPacket(state)
	if err != nil {
		return StorySession{}, err
	}
	nodeID := strings.TrimSpace(state.CurrentNodeID)
	if nodeID == "" {
		nodeID = entryNodeID(packet)
	}
	return StorySession{
		Workspace: ws,
		State:     state,
		Bible:     bible,
		Packet:    packet,
		NodeID:    nodeID,
		Bundle: story.SeedBundle{
			Bible:  bible,
			Packet: packet,
		},
	}, nil
}

func entryNodeID(packet scene.StoryPacket) string {
	if packet.EntryNodeID != "" {
		return packet.EntryNodeID
	}
	if node, err := packet.EntryNode(); err == nil {
		return node.ID
	}
	return ""
}

func summaryTime(summary StorySummary) time.Time {
	for _, value := range []string{summary.UpdatedAt, summary.CreatedAt} {
		if t, err := time.Parse(time.RFC3339, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
