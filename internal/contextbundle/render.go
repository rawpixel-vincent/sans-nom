package contextbundle

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func (c NarratorContext) RenderText() string {
	var b strings.Builder
	writeSection(&b, "ROLE BOUNDARY")
	fmt.Fprintf(&b, "You are the Narrator. Rewrite only the selected scene node into player-facing prose.\n")
	fmt.Fprintf(&b, "Do not expose Writer drafts, unused branches, private actor interiors, or planning notes.\n")
	fmt.Fprintf(&b, "Lore references are creative influence only; do not import source-canon names, places, factions, or events as facts in the new story.\n")

	writeStoryBibleDoctrine(&b, c.StoryBible)

	writeSection(&b, "CURRENT SCENE STATE")
	writeJSON(&b, c.SceneState)

	writeSection(&b, "PLAYER DECISION")
	fmt.Fprintln(&b, decisionText(c.PlayerDecision))

	writeSection(&b, "SELECTED NODE")
	writeJSON(&b, c.SelectedNode)

	writeActorContexts(&b, c.ActorContexts)
	writeLoreGroups(&b, groupLoreByKind(c.LoreHits))

	writeStringList(&b, "RECENT WITNESSED CANON", c.RecentScenes)
	writeRepresentations(&b, "DIRECTIONAL REPRESENTATIONS", c.Representations)
	writeStringList(&b, "MEMORY AND WITNESS RULES", c.WitnessRules)
	writeStringList(&b, "WARNINGS", c.Warnings)

	return capRenderedText(strings.TrimSpace(b.String()), c.MaxRenderChars)
}

func (c WriterContext) RenderText() string {
	var b strings.Builder
	writeSection(&b, "ROLE BOUNDARY")
	fmt.Fprintf(&b, "You are the Creative Writer. Plan the next bounded packet; the packet tree is planning material, not canon.\n")
	fmt.Fprintf(&b, "The Narrator owns final prose. Honcho receives only rolled-out witnessed canon, never unused branches.\n")
	fmt.Fprintf(&b, "Lore references encode SF-comics taste and passion; create new story content instead of copying source canon.\n")

	writeStoryBibleDoctrine(&b, c.StoryBible)

	writeSection(&b, "CURRENT SCENE STATE")
	writeJSON(&b, c.SceneState)

	writeSection(&b, "LATEST ROLLOUT AND DECISION")
	fmt.Fprintln(&b, decisionText(c.PlayerDecision))

	writeSection(&b, "SELECTED NODE")
	fmt.Fprintln(&b, c.SelectedNodeID)

	writeSection(&b, "CURRENT BOUNDED PACKET")
	writeJSON(&b, c.Packet)

	writeStringList(&b, "RECENT WITNESSED CANON", c.RecentScenes)
	writeRepresentations(&b, "DIRECTIONAL REPRESENTATIONS", c.Representations)
	writeLoreGroups(&b, c.LoreByKind)
	writeActorManifest(&b, c.ActorManifest)

	writeSection(&b, "PLANNING CONSTRAINTS")
	fmt.Fprintln(&b, "- Revise only the current bounded packet.")
	fmt.Fprintln(&b, "- StoryPacket closure rule: every choice target_node_id must point to a node in this same packet; future packet handoffs must be terminal nodes with revision_notes, not missing target ids.")
	fmt.Fprintln(&b, "- Keep all branches outside Honcho until the Narrator rolls one out.")
	fmt.Fprintln(&b, "- Use actor cards and directional representations only for original story actors explicitly present in the packet.")
	fmt.Fprintln(&b, "- Treat lore references as influence material, not as candidate actor inventory or canon to reproduce.")
	fmt.Fprintln(&b, "- Do not infer council mechanics, resurrection roster assumptions, or global shared knowledge.")

	writeStringList(&b, "WARNINGS", c.Warnings)

	return capRenderedText(strings.TrimSpace(b.String()), c.MaxRenderChars)
}

func writeStoryBibleDoctrine(b *strings.Builder, value *story.StoryBible) {
	if value == nil {
		return
	}
	writeSection(b, "STORY BIBLE DOCTRINE")
	fmt.Fprintf(b, "title: %s\n", value.Title)
	fmt.Fprintf(b, "premise: %s\n", value.Premise)
	fmt.Fprintf(b, "player: %s (%s)\n", value.PlayerCharacter.Name, value.PlayerCharacter.PeerID)
	fmt.Fprintf(b, "player_world_connection: %s\n", value.PlayerWorldConnection)
	for _, law := range value.OriginalLaws {
		fmt.Fprintf(b, "law %s (%s): %s\n", law.ID, law.Name, law.Description)
		fmt.Fprintf(b, "enforcement: %s\n", law.Enforcement)
		fmt.Fprintf(b, "why_profound: %s\n", law.WhyProfound)
		fmt.Fprintf(b, "not_merely_cruel: %s\n", law.NotMerelyCruel)
	}
	writeInlineList(b, "world_basics", value.WorldBasics)
	writeInlineList(b, "tone_doctrine", value.ToneDoctrine)
	writeInlineList(b, "first_turn_exposition", value.FirstTurnExposition)
	if value.ResumeSummary != "" {
		fmt.Fprintf(b, "resume_summary: %s\n", value.ResumeSummary)
	}
	if len(value.ForbiddenLiteralCanonTerms) > 0 {
		fmt.Fprintf(b, "forbidden_literal_canon_terms: %s\n", strings.Join(value.ForbiddenLiteralCanonTerms, ", "))
	}
}

func writeInlineList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(b, "- %s\n", strings.TrimSpace(value))
		}
	}
}

func writeSection(b *strings.Builder, title string) {
	if b.Len() > 0 {
		fmt.Fprintln(b)
		fmt.Fprintln(b)
	}
	fmt.Fprintf(b, "## %s\n", title)
}

func writeJSON(b *strings.Builder, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(b, "%v\n", value)
		return
	}
	fmt.Fprintln(b, string(data))
}

func writeStringList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	writeSection(b, title)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func writeRepresentations(b *strings.Builder, title string, reps []Representation) {
	if len(reps) == 0 {
		return
	}
	reps = append([]Representation(nil), reps...)
	sort.SliceStable(reps, func(i, j int) bool {
		if reps[i].Observer == reps[j].Observer {
			return reps[i].Observed < reps[j].Observed
		}
		return reps[i].Observer < reps[j].Observer
	})
	writeSection(b, title)
	for _, rep := range reps {
		fmt.Fprintf(b, "### %s -> %s\n%s\n", rep.Observer, rep.Observed, strings.TrimSpace(rep.Text))
	}
}

func writeActorContexts(b *strings.Builder, actors []ActorContext) {
	if len(actors) == 0 {
		return
	}
	actors = append([]ActorContext(nil), actors...)
	sort.SliceStable(actors, func(i, j int) bool {
		if actors[i].ActorID == actors[j].ActorID {
			return actors[i].Mode < actors[j].Mode
		}
		return actors[i].ActorID < actors[j].ActorID
	})
	writeSection(b, "ACTOR CONTEXTS")
	for _, actor := range actors {
		fmt.Fprintf(b, "### %s\n", actor.ActorID)
		fmt.Fprintf(b, "mode: %s\n", actor.Mode)
		if actor.Reason != "" {
			fmt.Fprintf(b, "reason: %s\n", actor.Reason)
		}
		if len(actor.Targets) > 0 {
			fmt.Fprintf(b, "targets: %s\n", strings.Join(actor.Targets, ", "))
		}
		fmt.Fprintf(b, "card_source: %s\n", actor.CardSource)
		fmt.Fprintln(b, "card:")
		for _, line := range strings.Split(actor.Card, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Fprintf(b, "- %s\n", line)
			}
		}
		writeInlineRepresentations(b, actor.Representations)
		writeInlineLore(b, actor.LoreHits)
		if len(actor.Warnings) > 0 {
			fmt.Fprintln(b, "warnings:")
			for _, warning := range actor.Warnings {
				fmt.Fprintf(b, "- %s\n", warning)
			}
		}
	}
}

func writeInlineRepresentations(b *strings.Builder, reps []Representation) {
	if len(reps) == 0 {
		return
	}
	reps = append([]Representation(nil), reps...)
	sort.SliceStable(reps, func(i, j int) bool {
		if reps[i].Observer == reps[j].Observer {
			return reps[i].Observed < reps[j].Observed
		}
		return reps[i].Observer < reps[j].Observer
	})
	fmt.Fprintln(b, "representations:")
	for _, rep := range reps {
		fmt.Fprintf(b, "- %s -> %s: %s\n", rep.Observer, rep.Observed, strings.TrimSpace(rep.Text))
	}
}

func writeInlineLore(b *strings.Builder, hits []LoreHit) {
	if len(hits) == 0 {
		return
	}
	hits = append([]LoreHit(nil), hits...)
	sortLore(hits)
	fmt.Fprintln(b, "scene-valid lore:")
	for _, hit := range hits {
		fmt.Fprintf(b, "- %.3f %s %s: %s\n", hit.Score, hit.ID, hit.Source, strings.TrimSpace(hit.Text))
	}
}

func writeLoreGroups(b *strings.Builder, byKind map[string][]LoreHit) {
	if len(byKind) == 0 {
		return
	}
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	writeSection(b, "CREATIVE INFLUENCE REFERENCES")
	for _, kind := range kinds {
		hits := append([]LoreHit(nil), byKind[kind]...)
		sortLore(hits)
		fmt.Fprintf(b, "### %s\n", kind)
		for _, hit := range hits {
			fmt.Fprintf(b, "- %.3f %s %s\n", hit.Score, hit.ID, hit.Source)
			if text := strings.TrimSpace(hit.Text); text != "" {
				fmt.Fprintf(b, "  %s\n", text)
			}
		}
	}
}

func writeActorManifest(b *strings.Builder, actors []ActorManifestItem) {
	if len(actors) == 0 {
		return
	}
	actors = append([]ActorManifestItem(nil), actors...)
	sort.SliceStable(actors, func(i, j int) bool {
		if actors[i].Explicit != actors[j].Explicit {
			return actors[i].Explicit
		}
		return actors[i].PeerID < actors[j].PeerID
	})
	writeSection(b, "STORY ACTOR MANIFEST")
	for _, actor := range actors {
		marker := ""
		if actor.Explicit {
			marker = " explicit"
		}
		fmt.Fprintf(b, "- %s%s", actor.PeerID, marker)
		if actor.DisplayName != "" {
			fmt.Fprintf(b, " (%s)", actor.DisplayName)
		}
		if actor.Role != "" {
			fmt.Fprintf(b, ": %s", actor.Role)
		}
		if actor.Reason != "" {
			fmt.Fprintf(b, " [%s]", actor.Reason)
		}
		fmt.Fprintln(b)
	}
}

func capRenderedText(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	notice := "\n\n[context truncated at render cap]"
	if max <= len(notice) {
		return text[:max]
	}
	return strings.TrimSpace(text[:max-len(notice)]) + notice
}
