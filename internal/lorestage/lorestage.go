package lorestage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
)

func ScenarioContractSeedMode(pack lorepack.Pack) bool {
	return strings.TrimSpace(pack.GameplayMode) != "" && strings.TrimSpace(pack.GameSynopsis) != ""
}

func ScenarioContractTextForPack(packID string) string {
	pack, err := lorepack.GetPack(runutil.RepoRoot(), runutil.FirstNonEmpty(packID, lorepack.DefaultPackID()))
	if err != nil {
		return ""
	}
	return pack.ScenarioContractText()
}

func MasterLoreSystemPromptForPack(pack lorepack.Pack, base string) string {
	text, err := pack.MasterLoreText()
	if err != nil {
		return strings.TrimSpace(base)
	}
	return prompt.WithMasterLoreSystemPrompt(base, pack.ID, pack.Label, text)
}

func MasterLoreSystemPromptForPackID(packID string, base string) string {
	pack, err := lorepack.GetPack(runutil.RepoRoot(), runutil.FirstNonEmpty(packID, lorepack.DefaultPackID()))
	if err != nil {
		return strings.TrimSpace(base)
	}
	return MasterLoreSystemPromptForPack(pack, base)
}

func MasterLoreSystemPromptFromScope(scope agentfs.Scope, base string) string {
	if strings.TrimSpace(scope.Root) == "" {
		return strings.TrimSpace(base)
	}
	path, err := scope.Abs("lorepack/LORE.md")
	if err != nil {
		return strings.TrimSpace(base)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return strings.TrimSpace(base)
	}
	return prompt.WithMasterLoreSystemPrompt(base, "", "", string(data))
}

func StageLorePackForAgent(scope agentfs.Scope, pack lorepack.Pack) (agentfs.Scope, error) {
	if strings.TrimSpace(scope.Root) == "" || strings.TrimSpace(pack.Path) == "" {
		return scope, nil
	}
	if err := scope.Ensure(); err != nil {
		return scope, err
	}
	if stagedRoot, err := scope.Abs("lorepack"); err == nil {
		if err := os.RemoveAll(stagedRoot); err != nil {
			return scope, fmt.Errorf("clear staged lorepack %q: %w", pack.ID, err)
		}
	}
	if err := stageCuratedLorePack(scope, pack); err != nil {
		return scope, fmt.Errorf("stage active lorepack %q: %w", pack.ID, err)
	}
	if _, err := scope.StageText("lorepack/navigation.md", lorepackNavigationGuide(pack)); err != nil {
		return scope, fmt.Errorf("stage active lorepack navigation %q: %w", pack.ID, err)
	}
	return scope.WithVisiblePaths("lorepack"), nil
}

func stageCuratedLorePack(scope agentfs.Scope, pack lorepack.Pack) error {
	base := filepath.Dir(pack.Path)
	if _, err := scope.StageFile(pack.Path, "lorepack/lore-pack.json"); err != nil {
		return err
	}
	if err := stagePackRelIfExists(scope, base, pack.MasterLoreRelPath()); err != nil {
		return err
	}
	if err := stagePackRelIfExists(scope, base, packArtifactRel(pack, "wiki", "wiki/lore.md")); err != nil {
		return err
	}
	if err := stagePackRelIfExists(scope, base, packArtifactRel(pack, "links", "links/matrix.md")); err != nil {
		return err
	}
	comicsRoot := packArtifactRel(pack, "comics", "comics")
	for _, rel := range []string{
		filepath.ToSlash(filepath.Join(comicsRoot, "manifest.json")),
		filepath.ToSlash(filepath.Join(comicsRoot, "peer-cards", "context-entrypoints.json")),
		filepath.ToSlash(filepath.Join(packArtifactRel(pack, "index", "index"), "manifest.json")),
	} {
		if err := stagePackRelIfExists(scope, base, rel); err != nil {
			return err
		}
	}
	for _, pattern := range []string{
		filepath.ToSlash(filepath.Join(comicsRoot, "volumes", "*.json")),
		filepath.ToSlash(filepath.Join(comicsRoot, "characters", "*.json")),
		filepath.ToSlash(filepath.Join(comicsRoot, "pages", "*.json")),
		filepath.ToSlash(filepath.Join(comicsRoot, "peer-cards", "cards", "accepted", "*.json")),
	} {
		if err := stagePackGlob(scope, base, pattern); err != nil {
			return err
		}
	}
	return nil
}

func packArtifactRel(pack lorepack.Pack, key string, fallback string) string {
	if pack.ArtifactRoots != nil {
		if value := strings.TrimSpace(pack.ArtifactRoots[key]); value != "" {
			return filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
		}
	}
	return fallback
}

func stagePackRelIfExists(scope agentfs.Scope, base string, rel string) error {
	rel, err := cleanPackRel(rel)
	if err != nil {
		return err
	}
	source := filepath.Join(base, filepath.FromSlash(rel))
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected lorepack file, got directory: %s", rel)
	}
	_, err = scope.StageFile(source, filepath.ToSlash(filepath.Join("lorepack", filepath.FromSlash(rel))))
	return err
}

func stagePackGlob(scope agentfs.Scope, base string, relPattern string) error {
	relPattern, err := cleanPackRel(relPattern)
	if err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(base, filepath.FromSlash(relPattern)))
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for _, source := range matches {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if info.IsDir() {
			continue
		}
		rel, err := filepath.Rel(base, source)
		if err != nil {
			return err
		}
		target := filepath.ToSlash(filepath.Join("lorepack", rel))
		if _, err := scope.StageFile(source, target); err != nil {
			return err
		}
	}
	return nil
}

func cleanPackRel(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errors.New("lorepack relative path is required")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("lorepack path escapes pack root: %s", rel)
	}
	return filepath.ToSlash(clean), nil
}

func StageNarratorLorePackForAgent(scope agentfs.Scope, pack lorepack.Pack) (agentfs.Scope, error) {
	if strings.TrimSpace(scope.Root) == "" || strings.TrimSpace(pack.Path) == "" {
		return scope, nil
	}
	if err := scope.Ensure(); err != nil {
		return scope, err
	}
	if _, err := scope.StageFile(pack.Path, "lorepack/lore-pack.json"); err != nil {
		return scope, fmt.Errorf("stage active lorepack contract %q: %w", pack.ID, err)
	}
	if _, err := scope.StageFile(pack.MasterLorePath(), "lorepack/LORE.md"); err != nil {
		return scope, fmt.Errorf("stage active lorepack master lore %q: %w", pack.ID, err)
	}
	if _, err := scope.StageText("lorepack/contract.md", pack.ScenarioContractText()+"\n"); err != nil {
		return scope, err
	}
	if _, err := scope.StageText("lorepack/relevant-excerpts.md", narratorLorepackExcerpts(pack)); err != nil {
		return scope, err
	}
	if _, err := scope.StageText("lorepack/source-anchors.md", lorepackSourceAnchors(pack)); err != nil {
		return scope, err
	}
	if _, err := scope.StageText("lorepack/navigation.md", narratorLorepackNavigationGuide(pack)); err != nil {
		return scope, err
	}
	return scope.WithVisiblePaths("lorepack"), nil
}

func StageLorePackForRoleByID(scope agentfs.Scope, role string, packID string) (agentfs.Scope, error) {
	pack, err := lorepack.GetPack(runutil.RepoRoot(), runutil.FirstNonEmpty(packID, lorepack.DefaultPackID()))
	if err != nil {
		return scope, err
	}
	if role == "narrator" {
		return StageNarratorLorePackForAgent(scope, pack)
	}
	return StageLorePackForAgent(scope, pack)
}

func narratorLorepackExcerpts(pack lorepack.Pack) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Narrator Lorepack Excerpts: %s (%s)\n\n", runutil.FirstNonEmpty(pack.Label, pack.ID), runutil.FirstNonEmpty(pack.ID, "unknown"))
	fmt.Fprintln(&b, "Use these excerpts before reading any other lorepack file. They are staged to keep narration scene-local.")
	fmt.Fprintf(&b, "\n## Gameplay Mode\n%s\n", strings.TrimSpace(pack.GameplayMode))
	fmt.Fprintf(&b, "\n## Synopsis\n%s\n", strings.TrimSpace(pack.GameSynopsis))
	fmt.Fprintf(&b, "\n## Protagonist Role\n%s\n", strings.TrimSpace(pack.ProtagonistRole))
	fmt.Fprintf(&b, "\n## Opening Situation\n%s\n", strings.TrimSpace(pack.OpeningSituation))
	fmt.Fprintf(&b, "\n## Initial Pressure\n%s\n", strings.TrimSpace(pack.InitialPressure))
	writeMarkdownList(&b, "Must Not Violate", pack.MustNotViolate)
	return b.String()
}

func lorepackSourceAnchors(pack lorepack.Pack) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Source Anchors: %s (%s)\n\n", runutil.FirstNonEmpty(pack.Label, pack.ID), runutil.FirstNonEmpty(pack.ID, "unknown"))
	anchor := pack.CanonAnchor
	if anchor.Empty() {
		fmt.Fprintln(&b, "No explicit canon anchors were authored for this pack.")
		return b.String()
	}
	if len(anchor.SourceVolumes) > 0 {
		var volumes []string
		for _, volume := range anchor.SourceVolumes {
			volumes = append(volumes, fmt.Sprintf("%d", volume))
		}
		fmt.Fprintf(&b, "## Source Volumes\n%s\n", strings.Join(volumes, ", "))
	}
	writeMarkdownList(&b, "Source Events", anchor.SourceEvents)
	writeMarkdownList(&b, "Source Characters", anchor.SourceCharacters)
	writeMarkdownList(&b, "Must Include Canon Facts", anchor.MustIncludeCanonFacts)
	writeMarkdownList(&b, "Allowed Original Insertions", anchor.AllowedOriginalInsertions)
	return b.String()
}

func lorepackNavigationGuide(pack lorepack.Pack) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Lorepack Navigation: %s (%s)\n\n", runutil.FirstNonEmpty(pack.Label, pack.ID), runutil.FirstNonEmpty(pack.ID, "unknown"))
	fmt.Fprintln(&b, "Use this file before drafting. The lorepack is staged under lorepack/ inside your workspace; paths below are workspace-relative.")
	fmt.Fprintln(&b, "\n## Minimum Orientation Before Writing")
	fmt.Fprintln(&b, "1. Treat lorepack/LORE.md as the master lore baseline. It is also injected into system context when available, so use it as stable background rather than re-summarizing it.")
	fmt.Fprintln(&b, "2. Read lorepack/lore-pack.json for gameplay_mode, game_synopsis, protagonist_role, opening_situation, initial_pressure, story_lines, avoid-drift notes, canon anchors, and any choice guidance.")
	fmt.Fprintln(&b, "3. Read lorepack/wiki/lore.md for broad source overview, terminology, characters, relationships, conflicts, and recurring story pressures.")
	fmt.Fprintln(&b, "4. Read lorepack/comics/manifest.json to understand available page ids, volumes, and source image coverage. Use it to choose files; do not treat it as story summary.")
	fmt.Fprintln(&b, "5. Read at least two relevant lorepack/comics/volumes/volume-XX.json files. Volume summaries are the best first stop for chronology, locations, lore laws, major events, motifs, and page ids.")
	fmt.Fprintln(&b, "6. Read at least one matching lorepack/comics/characters/characters-volume-XX.json file for who appears in that volume, relationship pressure, role, and character function.")
	fmt.Fprintln(&b, "7. Open lorepack/comics/pages/<page_id>.json only after a volume summary or search points to a specific scene detail.")
	fmt.Fprintln(&b, "8. If fidelity remains uncertain, ask_lore_expert with the exact uncertainty and source files you already checked.")
	fmt.Fprintln(&b, "\n## File Map")
	fmt.Fprintln(&b, "- lorepack/LORE.md: master lore summary; final lorepack build artifact and system-prompt baseline.")
	fmt.Fprintln(&b, "- lorepack/lore-pack.json: authored gameplay notes and source-fidelity boundaries.")
	fmt.Fprintln(&b, "- lorepack/wiki/lore.md: broad source overview; useful for names, relationships, conflicts, customs, and recurring pressure.")
	fmt.Fprintln(&b, "- lorepack/comics/volumes/volume-XX.json: volume-level summaries; use before page files.")
	fmt.Fprintln(&b, "- lorepack/comics/characters/characters-volume-XX.json: volume-level cast and role records.")
	fmt.Fprintln(&b, "- lorepack/comics/pages/<page_id>.json: page-level evidence; use for precise events, lines, staging, and object details.")
	fmt.Fprintln(&b, "- lorepack/comics/peer-cards/cards/accepted/*.json: character authority cards when the pack includes them.")
	fmt.Fprintln(&b, "- lorepack/comics/manifest.json: page catalog; choose page ids and volumes from here.")
	fmt.Fprintln(&b, "- lorepack/index/manifest.json: retrieval/index coverage; use to understand what the LoreExpert can see, not as plot summary.")
	fmt.Fprintln(&b, "\n## Search Guidance")
	fmt.Fprintln(&b, "- Use the Grep tool or rg. Do not use shell grep; it may be blocked by the read-only tool allowlist.")
	fmt.Fprintln(&b, "- Good focused searches name the concrete thing you need: a character, place, object, conflict, relationship, event, visual motif, or source term from the current scene.")
	fmt.Fprintln(&b, "- Search source terms only when you need a concrete anchor. Do not let one search hit become the whole premise.")
	fmt.Fprintln(&b, "- Prefer searches over directories that are already staged, for example lorepack/wiki/lore.md and lorepack/comics/volumes.")
	fmt.Fprintln(&b, "\n## Useful Read-Only Commands")
	fmt.Fprintln(&b, "- rg --files lorepack/comics/volumes")
	fmt.Fprintln(&b, "- rg --files lorepack/comics/characters")
	fmt.Fprintln(&b, "- rg -n \"character|place|object|conflict|relationship|event\" lorepack/wiki/lore.md lorepack/comics/volumes")
	fmt.Fprintln(&b, "- cat lorepack/comics/volumes/volume-01.json")
	fmt.Fprintln(&b, "- cat lorepack/comics/characters/characters-volume-01.json")
	fmt.Fprintln(&b, "\n## Anti-Patterns")
	fmt.Fprintln(&b, "- Do not draft after reading only lorepack/lore-pack.json.")
	fmt.Fprintln(&b, "- Do not browse random page files before choosing relevant volumes.")
	fmt.Fprintln(&b, "- Do not use one isolated page as the whole story contract.")
	fmt.Fprintln(&b, "- Do not dump unused canon into the seed. Use source detail to support playable pressure, choices, and consequence.")
	fmt.Fprintln(&b, "- Do not replace the gameplay contract with broad mood words.")
	writeNavigationAnchors(&b, pack)
	return b.String()
}

func narratorLorepackNavigationGuide(pack lorepack.Pack) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Narrator Lorepack Navigation: %s (%s)\n\n", runutil.FirstNonEmpty(pack.Label, pack.ID), runutil.FirstNonEmpty(pack.ID, "unknown"))
	fmt.Fprintln(&b, "Use staged lorepack files for scene fidelity, not source dumping.")
	fmt.Fprintln(&b, "\n## Read Order")
	fmt.Fprintln(&b, "1. lorepack/LORE.md is already injected into system context when available; use it as stable source-lore background, not narration text.")
	fmt.Fprintln(&b, "2. lorepack/contract.md for the scenario pressure and hard boundaries.")
	fmt.Fprintln(&b, "3. lorepack/relevant-excerpts.md for the current playable synopsis, role, pressure, and choice principles.")
	fmt.Fprintln(&b, "4. lorepack/source-anchors.md for specific canon anchor facts when the scene needs them.")
	fmt.Fprintln(&b, "5. lorepack/lore-pack.json only if a structured field is missing from the markdown summaries.")
	fmt.Fprintln(&b, "\n## Use")
	fmt.Fprintln(&b, "- Let source detail appear through action, local behavior, material pressure, household reaction, and consequence.")
	fmt.Fprintln(&b, "- Keep narration scene-local. Do not recite contract fields or unused canon.")
	writeNavigationAnchors(&b, pack)
	return b.String()
}

func writeNavigationAnchors(b *strings.Builder, pack lorepack.Pack) {
	anchor := pack.CanonAnchor
	if anchor.Empty() {
		return
	}
	fmt.Fprintln(b, "\n## Authored Source Anchors")
	if len(anchor.SourceVolumes) > 0 {
		var volumes []string
		for _, volume := range anchor.SourceVolumes {
			volumes = append(volumes, fmt.Sprintf("lorepack/comics/volumes/volume-%02d.json", volume))
		}
		fmt.Fprintf(b, "- Start with source volume summaries: %s\n", strings.Join(volumes, ", "))
	}
	writeMarkdownList(b, "Anchor Events", anchor.SourceEvents)
	writeMarkdownList(b, "Anchor Characters", anchor.SourceCharacters)
	writeMarkdownList(b, "Must Include Canon Facts", anchor.MustIncludeCanonFacts)
	writeMarkdownList(b, "Allowed Original Insertions", anchor.AllowedOriginalInsertions)
}

func writeMarkdownList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", title)
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintf(b, "- %s\n", value)
		}
	}
}
