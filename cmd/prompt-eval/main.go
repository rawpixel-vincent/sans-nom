package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
)

type probe struct {
	ID     string
	Prompt string
}

type motif struct {
	Name string
	RE   *regexp.Regexp
}

func main() {
	ctx := context.Background()
	root := firstNonEmpty(os.Getenv("SANS_NOM_REPO_ROOT"), ".")
	provider := firstNonEmpty(os.Getenv("PROBE_PROVIDER"), "MINIMAX")
	model := strings.TrimSpace(os.Getenv("PROBE_MODEL"))
	start := envInt("PROBE_START", 1)
	limit := envInt("PROBE_LIMIT", 0)
	flag.StringVar(&provider, "provider", provider, "Claude-compatible provider name, for example MINIMAX or ZAI")
	flag.StringVar(&model, "model", model, "model override for the selected provider")
	flag.IntVar(&start, "start", start, "1-based probe index to start from")
	flag.IntVar(&limit, "limit", limit, "maximum number of probes to run; 0 means all")
	flag.Parse()
	runner := scenarist.ProviderRunner{
		Provider: provider,
		Model:    model,
		EnvFiles: scenarist.DefaultEnvFiles(root),
		Timeout:  15 * time.Minute,
	}
	systemPrompt := `You are the Creative Writer for a Sans-Nom seed workshop. Answer the user's creative-writing test prompt directly.`
	probes := []probe{
		{
			ID: "01-minimal-story-starts",
			Prompt: `Invent 3 playable science-fantasy story starts.

For each: one person, one urgent want, one person opposing them, one place, and the first meaningful player choice.

Keep each under 180 words.`,
		},
		{
			ID: "02-human-pressure-first",
			Prompt: `Invent 3 playable science-fantasy story starts.

Start from a person trying to do one urgent thing today before someone else stops them. Make the relationship wound clear. Let the strange setting appear through the job, place, danger, or tools.

For each, include: protagonist, urgent want, opposing person, relationship wound, place, first player choice. Keep the premise playable before it becomes mythic.`,
		},
		{
			ID: "03-light-taste-guidance",
			Prompt: `You are creating a first playable vertical slice for an original science-fantasy comic story.

Invent 3 story starts. Prefer emotionally specific conflict, vivid images, active opposition, and choices that reveal character. The world should feel strange but lived-in; show that through work, tools, weather, architecture, danger, or social habit.

Avoid explaining a whole invented system. Keep each start focused on one urgent scene.`,
		},
		{
			ID: "04-source-dna-summary-lite",
			Prompt: `You are creating a first playable vertical slice for an original science-fantasy comic story.

Influence taste: intense family pressure, spectacular settings, bodies and technology under stress, refusal under impossible expectation, and consequences that feel material.

Invent 3 story starts. Each should begin with a person trying to do one urgent thing in a concrete place while a specific opponent interferes. Let the unusual elements arise from the work, danger, relationship, or place.`,
		},
		{
			ID: "05-current-style-descriptive",
			Prompt: `You are the Creative Writer for a Sans-Nom seed workshop.

Invent 4 original science-fantasy story germs. Each germ needs a vivid image, player desire, active opposition, character wound, relationship pressure, strange material condition, and first irreversible pressure.

Treat source lore as craft DNA only: pressure, contradiction, body/taboo, witness memory, operatic SF-comics rhythm, and philosophical consequence. Any binding doctrine belongs to the player character's lived order and should support conflict.`,
		},
		{
			ID: "06-heavy-motif-palette",
			Prompt: `You are the Creative Writer for a Sans-Nom seed workshop.

Invent 4 original operatic science-fantasy comics story germs. Use body law, taboo pressure, witness-bound memory, social enforcement, philosophical violence, family/function/refusal tension, binding doctrine, reader plausibility, material condition, and first irreversible pressure.

Each germ should include player desire, active opposition, relationship pressure, strange material condition, and a playable first choice.`,
		},
		{
			ID: "07-scenarist-grounded-transform",
			Prompt: `You are the Creative Writer for a seed workshop.

Use only these rough sparks as raw material; improve or compare them rather than inventing unrelated new premises.

Sparks:
1. A contract diver must recover a stolen engine part from a reef-hangar before her ex-mentor sells it.
2. A courier carrying illegal medicine across a heat-blasted causeway learns the patient has betrayed him.
3. A repair apprentice in a collapsing orbital market must choose between saving a rival's stall and catching the thief who framed her.

For each, make the relationship pressure sharper and add one science-fantasy element that grows naturally from the job or place.`,
		},
		{
			ID: "08-low-lore-material-openings",
			Prompt: `Invent three opening situations for a playable speculative adventure story.

Return three numbered sections: 1, 2, and 3.

Each starts with one practical job going wrong: repair, delivery, rescue, inspection, harvest, crossing, negotiation, theft, or escape.

Use grounded hazards: broken equipment, bad weather, missing cargo, corrupt supervisors, family pressure, dangerous terrain, unreliable tools, hostile customers, or local politics.

Use plain language and low lore density. In each situation, use at most one invented place name and one invented tool or device name. Prefer common job titles and ordinary personal names over named orders, factions, doctrines, titles, or grand institutions.

The unusual setting should come from material causes: infrastructure, ecology, transport, weather, resource scarcity, workplace tools, maintenance routines, money pressure, family pressure, or local trade. Make each opening understandable as a practical problem before it becomes a mystery.

Write 3-5 short paragraphs per situation, like a scene already in motion. Let the unusual setting appear through concrete work details.

End each situation with the first player choice.

After each situation, add:
- Why it could be playable
- What risk the Writer should fix later`,
		},
	}
	if start < 1 {
		start = 1
	}
	end := len(probes)
	if limit > 0 && start+limit-1 < end {
		end = start + limit - 1
	}
	motifs := []motif{
		{"bell/sound", regexp.MustCompile(`(?i)\b(bell|bells|chime|toll|song|sing|voice|choir|sound|music|hymn)\b`)},
		{"cult/rite", regexp.MustCompile(`(?i)\b(cult|rite|ritual|temple|priest|saint|sacred|covenant|ceremony)\b`)},
		{"debt/contract", regexp.MustCompile(`(?i)\b(debt|debts|contract|owed|inheritance|heir|dowry|ledger)\b`)},
		{"witness/testimony", regexp.MustCompile(`(?i)\b(witness|testimony|testify|memory|memories|remember|record|truth)\b`)},
		{"salt/mirror/drowned", regexp.MustCompile(`(?i)\b(salt|mirror|reflection|drown|drowned|sea|tide|cathedral)\b`)},
		{"living-body-architecture", regexp.MustCompile(`(?i)\b(living city|city.*body|body.*city|rib|ribs|bone|flesh|skin|vein|organ|stomach|womb|animal|beast)\b`)},
		{"law/doctrine", regexp.MustCompile(`(?i)\b(law|laws|doctrine|taboo|forbidden|legal|civic|court|tribunal)\b`)},
	}
	for i := start - 1; i < end; i++ {
		p := probes[i]
		fmt.Printf("\n===== PROVIDER %s PROBE %d/%d: %s =====\n", strings.ToUpper(provider), i+1, len(probes), p.ID)
		out, metadata, err := runner.RunScenaristWithMetadata(ctx, systemPrompt, p.Prompt)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		fmt.Printf("model=%s/%s\n\n", metadata.ModelProvider, metadata.Model)
		fmt.Println(strings.TrimSpace(out))
		fmt.Printf("\n--- motif counts: %s ---\n", motifSummary(out, motifs))
	}
}

func motifSummary(text string, motifs []motif) string {
	type count struct {
		Name  string
		Count int
	}
	counts := make([]count, 0, len(motifs))
	for _, motif := range motifs {
		counts = append(counts, count{Name: motif.Name, Count: len(motif.RE.FindAllStringIndex(text, -1))})
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Count == counts[j].Count {
			return counts[i].Name < counts[j].Name
		}
		return counts[i].Count > counts[j].Count
	})
	parts := make([]string, 0, len(counts))
	for _, count := range counts {
		if count.Count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", count.Name, count.Count))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
