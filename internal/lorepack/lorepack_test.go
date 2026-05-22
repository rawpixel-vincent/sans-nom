package lorepack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetPackLoadsMetabarons(t *testing.T) {
	pack, err := GetPack("../..", "metabarons")
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "metabarons" {
		t.Fatalf("id = %q", pack.ID)
	}
	if pack.Collection != "sans_nom_lore_metabarons" {
		t.Fatalf("collection = %q", pack.Collection)
	}
	if pack.GameplayMode != GameplayModeCanonInternalOriginal {
		t.Fatalf("gameplay_mode = %q", pack.GameplayMode)
	}
	contract := pack.ScenarioContract()
	if contract.GameSynopsis == "" || len(contract.MustHonor) == 0 || len(contract.ChoicePrinciples) == 0 {
		t.Fatalf("missing scenario contract: %#v", contract)
	}
	if pack.MasterLoreRelPath() != "LORE.md" {
		t.Fatalf("master lore rel path = %q", pack.MasterLoreRelPath())
	}
	masterLore, err := pack.MasterLoreText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(masterLore, "The Metabarons") || !strings.Contains(masterLore, "Bushitaka") {
		preview := masterLore
		if len(preview) > 200 {
			preview = preview[:200]
		}
		t.Fatalf("unexpected master lore summary:\n%s", preview)
	}
	contractText := pack.ScenarioContractText()
	for _, required := range []string{
		"Lorepack Scenario Contract",
		"gameplay_mode: canon_internal_original",
		"game_synopsis:",
		"must_honor:",
		"choice_principles:",
		"canon_anchor:",
		"allowed_original_insertions:",
	} {
		if !strings.Contains(contractText, required) {
			t.Fatalf("contract text missing %q:\n%s", required, contractText)
		}
	}
	orientationText := pack.ScenarioOrientationText()
	for _, required := range []string{
		"Lorepack Scenario Orientation",
		"contract_path: lorepack/lore-pack.json",
		"master_lore_path: lorepack/LORE.md",
		"pressure field for faithful play",
	} {
		if !strings.Contains(orientationText, required) {
			t.Fatalf("orientation text missing %q:\n%s", required, orientationText)
		}
	}
	for _, tooSpecific := range []string{
		"game_synopsis:",
		"must_honor:",
		"must_not_violate:",
	} {
		if strings.Contains(orientationText, tooSpecific) {
			t.Fatalf("orientation text should not inline full contract field %q:\n%s", tooSpecific, orientationText)
		}
	}
}

func TestListPacksRejectsDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "one", `"dup"`)
	writePack(t, root, "two", `"dup"`)
	if _, err := ListPacks(root); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestActivePackRequiresScenarioContract(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lore/packs", "empty")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
  "id": "empty",
  "label": "Empty",
  "description": "desc",
  "collection": "collection",
  "embedding_model": "voyage-4",
  "artifact_roots": {"index": "index"},
  "builder": "metabarons_comics",
  "smoke_queries": [],
  "forbidden_literal_terms": [],
  "status": "active"
}`
	if err := os.WriteFile(filepath.Join(path, "lore-pack.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GetPack(root, "empty"); err == nil {
		t.Fatal("expected active pack without scenario contract to fail")
	}
}

func TestDefaultPackID(t *testing.T) {
	if DefaultPackID() != "metabarons" {
		t.Fatalf("default = %q", DefaultPackID())
	}
}

func writePack(t *testing.T, root string, dir string, id string) {
	t.Helper()
	path := filepath.Join(root, "lore/packs", dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
  "id": ` + id + `,
  "label": "Pack",
  "description": "desc",
  "collection": "collection",
  "embedding_model": "voyage-4",
  "artifact_roots": {"index": "index"},
  "builder": "metabarons_comics",
  "smoke_queries": [],
  "forbidden_literal_terms": [],
  "status": "active",
  "gameplay_mode": "canon_internal_original",
  "game_synopsis": "A test heir must survive a dynasty trial.",
  "protagonist_role": "Dynasty heir",
  "opening_situation": "The heir is summoned to a trial chamber.",
  "initial_pressure": "The family demands a costly proof.",
  "story_lines": ["claim the inheritance"],
  "must_honor": ["dynasty pressure has consequences"],
  "must_not_violate": ["no consequence-free refusal"],
	"choice_principles": ["choices express stance toward duty and refusal"]
}`
	if err := os.WriteFile(filepath.Join(path, "lore-pack.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "LORE.md"), []byte("# Pack Lore\n\nTest master lore summary."), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCanonAdaptationRequiresCanonAnchor(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "adaptation", `"adaptation"`)
	path := filepath.Join(root, "lore/packs", "adaptation", "lore-pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(data), `"gameplay_mode": "canon_internal_original"`, `"gameplay_mode": "canon_adaptation"`, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GetPack(root, "adaptation"); err == nil || !strings.Contains(err.Error(), "canon_anchor is required") {
		t.Fatalf("expected missing canon_anchor error, got %v", err)
	}
}
