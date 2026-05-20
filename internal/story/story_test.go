package story

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestStoryBibleValidationRejectsMissingOriginalLaw(t *testing.T) {
	bundle := validBundle()
	bundle.Bible.OriginalLaws = nil

	if err := ValidateBundle(bundle); err == nil {
		t.Fatal("expected missing binding principle rejection")
	} else if !strings.Contains(err.Error(), "binding lore principle") {
		t.Fatalf("expected binding lore validation error, got: %v", err)
	}
}

func TestStoryBibleValidationRejectsMissingPlayerWorldConnection(t *testing.T) {
	bundle := validBundle()
	bundle.Bible.PlayerWorldConnection = ""

	if err := ValidateBundle(bundle); err == nil {
		t.Fatal("expected missing player-world connection rejection")
	}
}

func TestSeedBundleValidationAllowsLorepackTermsInFaithfulMode(t *testing.T) {
	bundle := validBundle()
	bundle.Bible.Premise = "Tonto waits in the Metabunker."

	if err := ValidateBundle(bundle); err != nil {
		t.Fatal(err)
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
		"narration",
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

func TestWorkspaceWritesReadableTurnNarration(t *testing.T) {
	root := t.TempDir()
	bundle := validBundle()
	ws, state, err := InitWorkspace(InitOptions{
		StoryID: "Book Story",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	record := TurnRecord{
		Turn:      1,
		StoryID:   state.StoryID,
		PacketID:  bundle.Packet.ID,
		NodeID:    bundle.Packet.EntryNodeID,
		Prose:     "The garden opened like a ledger.",
		Choices:   bundle.Packet.Nodes[0].Choices,
		Committed: true,
		NextDecision: scene.PlayerDecision{
			ChoiceID: "pay",
		},
	}
	rel, err := ws.SavePendingTurn(record)
	if err != nil {
		t.Fatal(err)
	}
	state.PendingTurnPath = rel
	state.Status = StatusAwaitingPlayerChoice
	if err := ws.SaveState(state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws.NarrationDir(), "pending-turn.md")); err != nil {
		t.Fatalf("missing pending narration: %v", err)
	}
	state, err = ws.RecordTurn(state, bundle.Packet, record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws.NarrationDir(), "pending-turn.md")); !os.IsNotExist(err) {
		t.Fatalf("pending narration was not cleared: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(ws.NarrationDir(), "turn-000001.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"# Turn 1", "The garden opened like a ledger.", "## Choices", "Choice: pay"} {
		if !strings.Contains(text, want) {
			t.Fatalf("narration missing %q:\n%s", want, text)
		}
	}
	files, err := ws.LoadNarrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "narration/turn-000001.md" || files[0].Text == "" {
		t.Fatalf("unexpected narration files: %#v", files)
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
			ForbiddenLiteralCanonTerms: []string{},
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
					{ID: "b3", Text: "A friend in the crowd tries to catch Rin's eye."},
					{ID: "b4", Text: "Rin chooses how to step into the rite before the real demand is spoken."},
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
