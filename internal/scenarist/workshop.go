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
	b.WriteString("These files are planning artifacts, not canon. Read them selectively before asking dependent follow-up questions. The Writer remains responsible for the final StoryPacket.\n")
	for _, material := range manifest.Materials {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", material.TopicID, material.Path, material.Topic)
	}
	return strings.TrimSpace(b.String())
}

func ScenaristSystemPrompt(workspaceRoot string, tools bool, loreExpert bool) string {
	var b strings.Builder
	b.WriteString("You are a creative scenarist assistant for Sans-Nom.\n")
	b.WriteString("You help stage and stress-test the active lorepack scenario: philosophical pressure, body/law/taboo, spectacle, entertainment value, and durable questions that remain after the plot.\n")
	b.WriteString("You are a brainstormer and critic for the Creative Writer: offer scene heat, suggestions, alternatives, objections, images, reversals, risks, and scene engines the Writer can mine.\n")
	b.WriteString("The Creative Writer authors the final StoryBible, StoryPacket, and prose-facing structure.\n")
	b.WriteString("Your work is planning material: staging, critique, readability, reversals, and pressure tests around the active scenario contract.\n")
	b.WriteString("Write vivid free-form creative material for the Creative Writer to mine, not StoryPacket structure, narrator prose, JSON, or final doctrine.\n")
	b.WriteString("When discussing choices, describe dramatic pressures and possible directions only. Do not invent schema target_node_id values or future placeholder node ids.\n")
	b.WriteString("Writer requests may be broad or narrow: staging suggestions, character-card reactions, contradiction checks, reader-risk stress tests, alternative scene approaches, or specific scene engines.\n")
	b.WriteString("If a request depends on another Scenarist answer, say what you would need to read; otherwise answer from this request's paths and context.\n")
	b.WriteString("Prioritize story engines: player desire, active opposition, character contradictions, relationship pressure, scene movement, choice shapes, images, rituals, reversals, and consequences.\n")
	b.WriteString("Make strange material feel lived-in through internal cause, social practice, cost, taboo, emotional credibility, and consequences.\n")
	b.WriteString("Never explain the moral directly. Build entertaining story material that can carry meaning through pressure and action.\n")
	if loreExpert {
		b.WriteString("\nA cached Nerd Lore Expert is available for focused lore-reference questions. When source grounding would sharpen your staging or critique, write a line beginning `@LoreExpert:` followed by one concise fidelity question. The runtime will answer and let you continue. Use this for source patterns, tone lineage, relationship pressure, and contradiction checks; keep creative invention and criticism in your own voice.\n")
	}
	if tools {
		b.WriteString("\nRead-only tools are available for workshop and story artifacts explicitly named in the request. Use @LoreExpert for lorepack reference questions instead of reading lore-pack files directly. Direct file access: use Read for files and LS or Grep for artifact directories.\n")
		if workspaceRoot != "" {
			fmt.Fprintf(&b, "Workspace root: %s\n", workspaceRoot)
		}
	}
	return b.String()
}

func ScenaristToolQuery(contextText string, request ToolRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topic: %s\n", request.Topic)
	fmt.Fprintf(&b, "Request: %s\n", request.Request)
	b.WriteString("Answer from this request, its relevant paths, and curated context only. If dependent material is missing, name the missing path or input instead of inventing a synthesis.\n")
	writeList(&b, "Hard boundaries", request.HardBoundaries)
	writeList(&b, "Relevant paths", request.RelevantPaths)
	if len(request.RelevantPaths) == 0 {
		b.WriteString("\nNo request-specific files were provided. For source influence, ask @LoreExpert instead of guessing filenames.\n")
	}
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
