package story

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestStoryBibleValidationRejectsMissingOriginalLaw(t *testing.T) {
	bundle := validBundle()
	bundle.Bible.OriginalLaws = nil

	if err := ValidateBundle(bundle); err == nil {
		t.Fatal("expected missing original law rejection")
	}
}

func TestStoryBibleValidationRejectsMissingPlayerWorldConnection(t *testing.T) {
	bundle := validBundle()
	bundle.Bible.PlayerWorldConnection = ""

	if err := ValidateBundle(bundle); err == nil {
		t.Fatal("expected missing player-world connection rejection")
	}
}

func TestSeedBundleValidationRejectsLiteralCanonTerms(t *testing.T) {
	bundle := validBundle()
	bundle.Bible.Premise = "Tonto waits in the Metabunker."

	if err := ValidateBundle(bundle); err == nil {
		t.Fatal("expected literal canon term rejection")
	}
}

func TestSeedPacketRejectsAllTerminalOpeningBranchesWithoutContinuationNotes(t *testing.T) {
	bundle := validBundle()
	for i := range bundle.Packet.Nodes {
		if bundle.Packet.Nodes[i].ID != "entry" {
			bundle.Packet.Nodes[i].RevisionNotes = nil
		}
	}

	if err := ValidateBundle(bundle); err == nil {
		t.Fatal("expected terminal branch rejection")
	}
}

func TestWorkspaceCreatesRequiredFilesAndResumes(t *testing.T) {
	root := t.TempDir()
	ws, state, err := InitWorkspace(InitOptions{
		StoryID:              "Test Story",
		Root:                 root,
		GroundLorePackID:     "metabarons",
		GroundLoreCollection: "ground-test",
		Replace:              true,
	}, validBundle())
	if err != nil {
		t.Fatal(err)
	}

	required := []string{
		"state.json",
		"bible.json",
		filepath.Join("packets", state.CurrentPacketID+".json"),
		filepath.Join("model-runs", "seed-bundle.json"),
		filepath.Join("index", "documents.jsonl"),
		filepath.Join("index", "manifest.json"),
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(ws.Dir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	loadedWS, loadedState, err := LoadWorkspace(root, "test-story")
	if err != nil {
		t.Fatal(err)
	}
	packet, err := loadedWS.LoadPacket(loadedState)
	if err != nil {
		t.Fatal(err)
	}
	if loadedState.StoryID != "test-story" || packet.ID != "packet-1" {
		t.Fatalf("unexpected resume state: %#v packet=%s", loadedState, packet.ID)
	}
	if loadedState.SourceLorePackID() != "metabarons" || loadedState.SourceLoreCollection() != "ground-test" {
		t.Fatalf("unexpected source lore: %#v", loadedState)
	}

	docs, err := loadedWS.ReadIndexDocuments()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, doc := range docs {
		kinds[doc.Kind] = true
		if doc.SourceSHA256 == "" || doc.TextSHA256 == "" || doc.PointID == "" {
			t.Fatalf("index doc missing hashes: %#v", doc)
		}
	}
	for _, kind := range []string{"story_bible", "story_law", "story_peer_card", "story_packet", "story_packet_node"} {
		if !kinds[kind] {
			t.Fatalf("missing index kind %s in %#v", kind, kinds)
		}
	}
}

func TestLoadWorkspaceBackfillsLegacyGroundLoreState(t *testing.T) {
	root := t.TempDir()
	ws, state, err := InitWorkspace(InitOptions{
		StoryID: "Legacy Story",
		Root:    root,
		Replace: true,
	}, validBundle())
	if err != nil {
		t.Fatal(err)
	}
	state.GroundLorePackID = ""
	state.GroundLoreCollection = ""
	state.GlobalLoreCollection = DefaultGlobalLoreCollection
	state.Status = ""
	if err := ws.SaveState(state); err != nil {
		t.Fatal(err)
	}

	_, loaded, err := LoadWorkspace(root, "legacy-story")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GroundLorePackID != DefaultGroundLorePackID {
		t.Fatalf("ground pack = %q", loaded.GroundLorePackID)
	}
	if loaded.GroundLoreCollection != DefaultGroundLoreCollection || loaded.SourceLoreCollection() != DefaultGroundLoreCollection {
		t.Fatalf("ground collection = %q source = %q", loaded.GroundLoreCollection, loaded.SourceLoreCollection())
	}
	if loaded.Status != StatusActive {
		t.Fatalf("status = %q", loaded.Status)
	}
}

func validBundle() SeedBundle {
	return SeedBundle{
		Bible: StoryBible{
			StoryID: "test-story",
			Title:   "The Debt Garden",
			Premise: "A child of civic gardeners must choose whether a living tax can preserve the dead without owning the living.",
			PlayerCharacter: PlayerCharacter{
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
			OriginalLaws: []OriginalLaw{{
				ID:             "living-tax",
				Name:           "Law of Living Tax",
				Description:    "A debt is settled only when a living capacity is marked by the civic garden.",
				SourcePattern:  "binding taboo structure",
				Enforcement:    "Public ledgers and witnesses refuse unmarked repayment.",
				WhyProfound:    "It protects proof for the powerless at bodily cost.",
				NotMerelyCruel: "It blocks forged mercy by the ruling houses.",
			}},
			Factions: []Faction{{
				ID:          "ledger-gardeners",
				Name:        "Ledger Gardeners",
				Description: "Public clerks who keep civic memory alive.",
			}},
			CoreRelationships:          []string{"Rin owes love and suspicion to the same household."},
			ToneDoctrine:               []string{"Body law should feel sacred before tactical."},
			SourceLoreInfluenceRefs:    []string{"source-law structure only"},
			ForbiddenLiteralCanonTerms: ForbiddenCanonTerms(),
			FirstTurnExposition:        []string{"world", "player", "connection", "law", "pressure"},
			ResumeSummary:              "Rin stands before the civic garden as the debt rite begins.",
			Metadata:                   map[string]string{"test": "true"},
		},
		Packet: scene.StoryPacket{
			ID:          "packet-1",
			EntryNodeID: "entry",
			ContextSnapshot: scene.ContextSnapshot{
				WorkspaceID: "test-story",
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
					{ID: "b3", Text: "The living tax is named before neighbors."},
					{ID: "b4", Text: "A parent record will vanish if Rin refuses."},
				},
				Choices: []scene.Choice{{
					ID:           "pay",
					Label:        "Pay the tax.",
					TargetNodeID: "pay",
				}, {
					ID:           "refuse",
					Label:        "Refuse the garden.",
					TargetNodeID: "refuse",
				}},
				MemoryIntents: []scene.MemoryIntent{{
					ID:        "law",
					Summary:   "The city demanded Rin pay living tax.",
					Policy:    scene.MemoryPolicyCanon,
					Witnesses: []string{"sans-nom"},
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
			}},
		},
	}
}
