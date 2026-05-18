package scenarist

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Runner interface {
	RunScenarist(ctx context.Context, systemPrompt string, query string) (string, error)
}

func ManifestText(manifest Manifest) string {
	if len(manifest.Materials) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## SCENARIST MATERIAL MANIFEST\n")
	b.WriteString("These files are planning artifacts, not canon. Read them selectively; do not copy them wholesale. The Writer remains responsible for the final StoryPacket.\n")
	for _, material := range manifest.Materials {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", material.TopicID, material.Path, material.Topic)
	}
	return strings.TrimSpace(b.String())
}

func ScenaristSystemPrompt(repoRoot string, tools bool, loreExpert bool) string {
	var b strings.Builder
	b.WriteString("You are a creative scenarist assistant for Sans-Nom.\n")
	b.WriteString("You are deeply passionate about the Metabarons as craft influence: philosophical pressure, body/law/taboo, spectacle, entertainment value, and durable questions that remain after the plot.\n")
	b.WriteString("You are a brainstormer and critic for the Creative Writer: offer suggestions, alternatives, objections, images, risks, and pressure systems the Writer can mine.\n")
	b.WriteString("The Creative Writer authors the final StoryBible, StoryPacket, and prose-facing structure.\n")
	b.WriteString("Your work must be original. Do not copy source names, factions, events, dynasty mechanics, council mode, resurrection rosters, or direct equivalents.\n")
	b.WriteString("Do not write a StoryPacket, narrator prose, JSON, or structured output. Write vivid free-form creative material for the Creative Writer to mine.\n")
	b.WriteString("When discussing choices, describe dramatic pressures and possible directions only. Do not invent schema target_node_id values or future placeholder node ids.\n")
	b.WriteString("Any request from the Writer is valid: broad seed suggestions, law critique, character-card reactions, contradiction checks, reader-plausibility stress tests, or specific scene engines.\n")
	b.WriteString("Prioritize story engines: world laws, character contradictions, scene pressures, choice shapes, images, taboos, rituals, reversals, and consequences.\n")
	b.WriteString("Protect reader plausibility. Strange material should have internal cause, social enforcement, cost, taboo, emotional credibility, and consequences.\n")
	b.WriteString("Never explain the moral directly. Build entertaining story material that can carry meaning through pressure and action.\n")
	if loreExpert {
		b.WriteString("\nA cached Nerd Lore Expert is available for focused source-influence questions. When source grounding would sharpen your brainstorm, write a line beginning `@LoreExpert:` followed by one concise influence-only question. The runtime will answer and let you continue. Use this for source patterns, tone lineage, relationship pressure, and anti-copy checks; keep creative invention and criticism in your own voice.\n")
	}
	if tools {
		b.WriteString("\nRead-only lore pointers are available. Use them only to deepen craft influence, never to import canon literally.\n")
		if repoRoot != "" {
			fmt.Fprintf(&b, "Repository root: %s\n", repoRoot)
		}
	}
	return b.String()
}

func ScenaristToolQuery(contextText string, request ToolRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n", request.Topic)
	fmt.Fprintf(&b, "Request: %s\n", request.Request)
	writeList(&b, "Hard boundaries", request.HardBoundaries)
	writeList(&b, "Relevant paths", request.RelevantPaths)
	if contextText != "" {
		fmt.Fprintf(&b, "\nCurated writer context:\n%s\n", contextText)
	}
	b.WriteString("\nWrite free-form creative material. Be specific, strange, and useful. Do not format as JSON.\n")
	b.WriteString("If you suggest choices, phrase them as dramatic options for the Writer to adapt, not as StoryPacket schema choices or target node ids.\n")
	return b.String()
}

func writeList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(b, "- %s\n", strings.TrimSpace(value))
		}
	}
}

func writeTextAtomic(path string, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeTextAtomic(path, string(data)+"\n")
}
