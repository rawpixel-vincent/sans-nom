package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

func main() {
	loadDotEnv(".env")

	packetPath := flag.String("packet", "testdata/packets/toy.json", "StoryPacket JSON path")
	nodeID := flag.String("node", "", "node id to roll out; defaults to packet entry node")
	choiceID := flag.String("choice", "", "selected choice id")
	freeText := flag.String("text", "", "free-text player input")
	useClaude := flag.Bool("claude", false, "use claude -p Sonnet adapters instead of static local adapters")
	refreshWriter := flag.Bool("refresh-writer", false, "run the Creative Writer asynchronously before rollout")
	loreSearch := flag.String("lore-search", "", "query the runtime lore retriever and exit")
	loreK := flag.Int("k", 5, "number of lore search results")
	bridgeURL := flag.String("bridge-url", os.Getenv("SANS_NOM_BRIDGE_URL"), "bridge sidecar URL")
	flag.Parse()

	if *loreSearch != "" {
		if err := runLoreSearch(context.Background(), *bridgeURL, *loreSearch, *loreK); err != nil {
			log.Fatal(err)
		}
		return
	}

	packet, err := openPacket(*packetPath)
	if err != nil {
		log.Fatal(err)
	}
	runner := claudeRunner()
	if *refreshWriter {
		job, err := writer.Service{
			Planner: writer.ClaudePlanner{Runner: runner},
		}.RefreshAsync(context.Background(), packet, decisionText(*choiceID, *freeText))
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), writer.DefaultAwaitTimeout())
		defer cancel()
		result := job.Await(ctx)
		if result.Err != nil {
			log.Fatal(result.Err)
		}
		packet = result.Packet
	}
	registry, err := actorRegistry()
	if err != nil {
		log.Fatal(err)
	}
	var actorInvoker actors.Invoker = actors.StaticInvoker{}
	var renderer narrator.Renderer = narrator.StaticRenderer{}
	if *useClaude {
		actorInvoker = actors.ClaudeInvoker{Runner: runner}
		renderer = narrator.ClaudeRenderer{Runner: runner}
	}
	svc := narrator.Service{
		Actors: &actors.Service{
			Registry: registry,
			Invoker:  actorInvoker,
		},
		Renderer: renderer,
	}
	rollout, err := svc.Rollout(context.Background(), packet, *nodeID, scene.PlayerDecision{
		ChoiceID: *choiceID,
		FreeText: *freeText,
	}, "phase0-cli")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(rollout.Draft.Prose)
	if len(rollout.Draft.Choices) > 0 {
		fmt.Println("\nChoices:")
		for _, choice := range rollout.Draft.Choices {
			fmt.Printf("- %s: %s\n", choice.ID, choice.Label)
		}
	}
	fmt.Printf("\nCommit prepared: packet=%s node=%s witnesses=%s events=%d\n",
		rollout.Commit.PacketID,
		rollout.Commit.NodeID,
		strings.Join(rollout.Commit.Witnesses, ","),
		len(rollout.Commit.Events),
	)
}

func runLoreSearch(ctx context.Context, bridgeURL string, query string, k int) error {
	retriever := lore.BridgeRetriever{
		Client: bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN")),
	}
	results, err := retriever.Search(ctx, query, k)
	if err != nil {
		return err
	}
	for _, result := range results {
		fmt.Printf("%.3f %s %s\n", result.Score, result.ID, result.Source)
		if text := preview(result.Text, 240); text != "" {
			fmt.Printf("  %s\n", text)
		}
	}
	return nil
}

func preview(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func claudeRunner() prompt.ClaudeRunner {
	model := os.Getenv("SANS_NOM_MODEL")
	if model == "" {
		model = "sonnet"
	}
	effort := os.Getenv("SANS_NOM_MODEL_EFFORT")
	if effort == "" {
		effort = "medium"
	}
	return prompt.ClaudeRunner{
		Model:   model,
		Effort:  effort,
		WorkDir: ".",
		Timeout: 3 * time.Minute,
	}
}

func decisionText(choiceID string, freeText string) string {
	if freeText != "" {
		return freeText
	}
	if choiceID != "" {
		return choiceID
	}
	return "observe"
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func openPacket(path string) (scene.StoryPacket, error) {
	file, err := os.Open(path)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	defer file.Close()
	return scene.DecodeStoryPacket(file)
}

func actorRegistry() (*actors.Registry, error) {
	registry, err := actors.LoadCards(filepath.Join("lore", "actors"))
	if err == nil && len(registry.IDs()) > 0 {
		return registry, nil
	}
	return actors.NewRegistry([]actors.Actor{{
		ID:          "tonto",
		DisplayName: "Tonto",
		Card:        "TRAIT: Tonto tells long witness-bound stories and never becomes a codex.",
	}, {
		ID:          "aghora",
		DisplayName: "Aghora",
		Card:        "TRAIT: Aghora's precision is her love.",
	}})
}
