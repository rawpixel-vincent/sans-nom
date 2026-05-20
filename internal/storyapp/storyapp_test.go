package storyapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func TestListStoriesSkipsInvalidAndSortsNewestFirst(t *testing.T) {
	root := t.TempDir()
	createStoryFixture(t, root, "older", "Older Title", "2026-01-01T00:00:00Z")
	createStoryFixture(t, root, "newer", "Newer Title", "2026-02-01T00:00:00Z")
	if err := os.MkdirAll(filepath.Join(root, "broken"), 0o755); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListStories(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries len = %d, want 2: %#v", len(summaries), summaries)
	}
	if summaries[0].StoryID != "newer" || summaries[1].StoryID != "older" {
		t.Fatalf("unexpected sort order: %#v", summaries)
	}
	if summaries[0].Label != "Newer Title" {
		t.Fatalf("label = %q, want title", summaries[0].Label)
	}
}

func TestListStoriesFallsBackToStoryIDLabel(t *testing.T) {
	root := t.TempDir()
	ws, state := createStoryFixture(t, root, "untitled", "Original Title", "2026-01-01T00:00:00Z")
	bible, err := ws.LoadBible()
	if err != nil {
		t.Fatal(err)
	}
	bible.Title = ""
	writeJSONForTest(t, filepath.Join(ws.Dir, "bible.json"), bible)
	writeJSONForTest(t, filepath.Join(ws.Dir, "state.json"), state)

	summaries, err := ListStories(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Label != "untitled" {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
}

func TestResumeStoryReturnsWorkspaceStateBibleAndPacket(t *testing.T) {
	root := t.TempDir()
	createStoryFixture(t, root, "resume-me", "Resume Me", "2026-01-01T00:00:00Z")

	session, err := ResumeStory(context.Background(), "resume-me", StoryActionOptions{StoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if session.State.StoryID != "resume-me" {
		t.Fatalf("state story id = %q", session.State.StoryID)
	}
	if session.Bible.Title != "Resume Me" {
		t.Fatalf("bible title = %q", session.Bible.Title)
	}
	if session.Packet.ID != "packet-resume-me" || session.NodeID != "entry" {
		t.Fatalf("unexpected packet/node: %s %s", session.Packet.ID, session.NodeID)
	}
}

func TestNewStoryUsesCallbacksAndReturnsSession(t *testing.T) {
	root := t.TempDir()
	var generated bool
	var seeded bool
	session, err := NewStory(context.Background(), StoryActionOptions{
		StoryID:              "new-one",
		StoryRoot:            root,
		GroundLorePackID:     "metabarons",
		GroundLoreCollection: "global-test",
		GenerateSeedBundle: func(_ context.Context, _ StoryActionOptions) (story.SeedBundle, error) {
			generated = true
			return validBundleForStory("new-one", "New One"), nil
		},
		SeedExternal: func(_ context.Context, session StorySession, bundle story.SeedBundle) error {
			seeded = true
			if session.State.StoryID != "new-one" || bundle.Bible.StoryID != "new-one" {
				t.Fatalf("external seed got wrong story ids: session=%s bundle=%s", session.State.StoryID, bundle.Bible.StoryID)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !generated || !seeded {
		t.Fatalf("callbacks generated=%v seeded=%v", generated, seeded)
	}
	if session.State.GroundLorePackID != "metabarons" || session.State.GroundLoreCollection != "global-test" {
		t.Fatalf("ground lore = %q %q", session.State.GroundLorePackID, session.State.GroundLoreCollection)
	}
}

func createStoryFixture(t *testing.T, root string, id string, title string, updatedAt string) (story.Workspace, story.StoryState) {
	t.Helper()
	ws, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: id,
		Root:    root,
		Replace: true,
	}, validBundleForStory(id, title))
	if err != nil {
		t.Fatal(err)
	}
	state.UpdatedAt = updatedAt
	writeJSONForTest(t, filepath.Join(ws.Dir, "state.json"), state)
	return ws, state
}

func validBundleForStory(id string, title string) story.SeedBundle {
	return story.SeedBundle{
		Bible: story.StoryBible{
			StoryID: id,
			Title:   title,
			Premise: "A keeper must decide whether civic memory can protect the dead without owning the living.",
			PlayerCharacter: story.PlayerCharacter{
				PeerID:      "sans-nom",
				Name:        "Rin",
				Description: "A young valve-keeper with a civic graft under one palm.",
				Desire:      "Rin wants to protect a parent record without becoming property.",
				Wound:       "Rin has learned that mercy arrives only after paperwork.",
				Boundary:    "Rin will not become an instrument with no private will.",
			},
			PlayerWorldConnection: "Rin's graft makes the city recognize them as both citizen and tool.",
			WorldBasics: []string{
				"The city records grief in public gardens.",
				"Debt law is performed by machines and crowds.",
				"Family memory can be protected only by witnessed sacrifice.",
			},
			OriginalLaws: []story.OriginalLaw{{
				ID:             "living-tax",
				Name:           "Law of Living Tax",
				Description:    "A debt is settled only when a living capacity is marked by the civic garden.",
				SourcePattern:  "binding taboo structure",
				Enforcement:    "Public ledgers and witnesses refuse unmarked repayment.",
				WhyProfound:    "It protects proof for the powerless at bodily cost.",
				NotMerelyCruel: "It blocks forged mercy by the ruling houses.",
			}},
			Factions: []story.Faction{{
				ID:          "ledger-gardeners",
				Name:        "Ledger Gardeners",
				Description: "Public clerks who keep civic memory alive.",
			}},
			CoreRelationships:          []string{"Rin owes love and suspicion to the same household."},
			ToneDoctrine:               []string{"Body law should feel sacred before tactical."},
			SourceLoreInfluenceRefs:    []string{"source-law structure only"},
			ForbiddenLiteralCanonTerms: []string{},
			FirstTurnExposition:        []string{"world", "player", "connection", "law", "pressure"},
			ResumeSummary:              "Rin stands before the civic garden as the debt rite begins.",
			Metadata:                   map[string]string{"test": "true"},
		},
		Packet: scene.StoryPacket{
			ID:          "packet-" + id,
			EntryNodeID: "entry",
			ContextSnapshot: scene.ContextSnapshot{
				WorkspaceID: id,
				SessionID:   "main",
				Location:    "Debt Garden",
				Time:        "opening bell",
			},
			Nodes: []scene.SceneNode{{
				ID:    "entry",
				Title: "The garden asks",
				Beats: []scene.Beat{
					{ID: "b1", Text: "The city opens its public garden for debt witness."},
					{ID: "b2", Text: "Rin stands with the graft hidden in one palm."},
					{ID: "b3", Text: "A friend in the crowd tries to catch Rin's eye."},
					{ID: "b4", Text: "Rin chooses how to enter before the real demand is spoken."},
				},
				Choices: []scene.Choice{{
					ID:           "enter_quiet",
					Label:        "Enter quietly.",
					TargetNodeID: "decision",
				}, {
					ID:           "enter_marked",
					Label:        "Show the graft.",
					TargetNodeID: "decision",
				}},
				MemoryIntents: []scene.MemoryIntent{{
					ID:        "law",
					Summary:   "Rin entered the civic garden before the debt rite.",
					Policy:    scene.MemoryPolicyCanon,
					Witnesses: []string{"sans-nom"},
				}},
			}, {
				ID:    "decision",
				Title: "The record is named",
				Beats: []scene.Beat{
					{Text: "The living tax is named before neighbors."},
					{Text: "A parent record will vanish if Rin refuses."},
					{Text: "The opposing clerk steps close enough to force a public answer."},
					{Text: "Rin must choose a value to damage."},
				},
				Choices: []scene.Choice{{
					ID:           "pay",
					Label:        "Pay the tax.",
					TargetNodeID: "pay",
				}, {
					ID:           "refuse",
					Label:        "Refuse the garden.",
					TargetNodeID: "refuse",
				}, {
					ID:           "accuse",
					Label:        "Accuse the clerk.",
					TargetNodeID: "accuse",
				}},
			}, {
				ID:      "pay",
				Title:   "Paid",
				Beats:   []scene.Beat{{Text: "The graft warms."}},
				Choices: []scene.Choice{},
				RevisionNotes: []string{
					"Writer continuation is required after this branch.",
				},
			}, {
				ID:      "refuse",
				Title:   "Refused",
				Beats:   []scene.Beat{{Text: "The ledgers stare back."}},
				Choices: []scene.Choice{},
				RevisionNotes: []string{
					"Writer continuation is required after this branch.",
				},
			}, {
				ID:      "accuse",
				Title:   "Accused",
				Beats:   []scene.Beat{{Text: "The clerk's seal cracks in public."}},
				Choices: []scene.Choice{},
				RevisionNotes: []string{
					"Writer continuation is required after this branch.",
				},
			}},
		},
	}
}

func writeJSONForTest(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
