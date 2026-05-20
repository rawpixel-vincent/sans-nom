package lorepack

import (
	"os"
	"path/filepath"
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
	if pack.GameplayMode != GameplayModeFaithfulCanonPrototype {
		t.Fatalf("gameplay_mode = %q", pack.GameplayMode)
	}
	contract := pack.ScenarioContract()
	if contract.GameSynopsis == "" || len(contract.MustHonor) == 0 || len(contract.ChoicePrinciples) == 0 {
		t.Fatalf("missing scenario contract: %#v", contract)
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

func TestActivePackRequiresFaithfulScenarioContract(t *testing.T) {
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
  "gameplay_mode": "faithful_canon_prototype",
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
}
