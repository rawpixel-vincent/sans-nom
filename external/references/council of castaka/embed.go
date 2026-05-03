package council

import (
	"embed"
	"fmt"
	"strings"
	"sync"
)

//go:embed metabarons/*.md metabarons/relations/*.md
var content embed.FS

// MemberDef holds a parsed character card and metadata.
type MemberDef struct {
	Key         string   // short name: "othon", "honorata", etc.
	Role        RoleName // config role constant
	DisplayName string   // e.g. "Honorata — La Trisaïeule"
	Card        string   // character prompt (content between ``` markers)
	Shadow      string   // Oda only: hidden shadow line injected into prompt
}

// memberSpec maps filename → (key, role).
var memberSpec = []struct {
	File string
	Key  string
	Role RoleName
}{
	{"metabarons/OthonVonSalza.md", "othon", RoleOthonVonSalza},
	{"metabarons/Honorata.md", "honorata", RoleHonorata},
	{"metabarons/AghnarVonSalza.md", "aghnar", RoleAghnarVonSalza},
	{"metabarons/Oda.md", "oda", RoleOda},
	{"metabarons/TeteDacier.md", "steelhead", RoleTeteDacier},
	{"metabarons/DonaVicentaGabrielaDeRokha.md", "vicenta", RoleDonaVicentaGabrielaDeRokha},
	{"metabarons/Aghora.md", "aghora", RoleAghora},
	{"metabarons/SansNom.md", "sans-nom", RoleSansNom},
}

// sectionToKey maps the canonical section headings in links.md to member keys.
var sectionToKey = map[string]string{
	"OTHON VON SALZA":                "othon",
	"HONORATA":                       "honorata",
	"AGHNAR VON SALZA":               "aghnar",
	"ODA":                            "oda",
	"TÊTE D'ACIER (STEELHEAD)":       "steelhead",
	"DOÑA VICENTA GABRIELA DE ROKHA": "vicenta",
	"AGHORA":                         "aghora",
	"SANS-NOM":                       "sans-nom",
}

// arrowNames maps the short names used in relationship arrows to keys.
var arrowNames = map[string]string{
	"Othon":     "othon",
	"Honorata":  "honorata",
	"Aghnar":    "aghnar",
	"Oda":       "oda",
	"Steelhead": "steelhead",
	"Vicenta":   "vicenta",
	"Aghora":    "aghora",
	"Sans-Nom":  "sans-nom",
}

var (
	parseOnce       sync.Once
	members         []MemberDef
	relations       map[string]map[string]string // fromKey → toKey → note
	invOneShot      string
	invPeerPress    string
	pressurePrompts map[string]string // memberKey → summoning pressure text
)

func ensureParsed() {
	parseOnce.Do(func() {
		members = parseMembers()
		relations = parseRelations()
		invOneShot = readFile("metabarons/invokation-one-shot.md")
		invPeerPress = readFile("metabarons/invokation-with-peer-pressure.md")
		pressurePrompts = parsePressurePrompts()
	})
}

// AllMembers returns the parsed character definitions.
func AllMembers() []MemberDef {
	ensureParsed()
	return members
}

// RelationshipNote returns the directional relationship note from → to.
func RelationshipNote(fromKey, toKey string) string {
	ensureParsed()
	if m, ok := relations[fromKey]; ok {
		return m[toKey]
	}
	return ""
}

// ArrowNameToKey returns the mapping from short names used in relationship arrows to member keys.
func ArrowNameToKey() map[string]string {
	// Return a copy to prevent mutation.
	cp := make(map[string]string, len(arrowNames))
	for k, v := range arrowNames {
		cp[k] = v
	}
	return cp
}

// InvocationTemplate returns the one-shot or peer-pressure invocation template.
func InvocationTemplate(peerPressure bool) string {
	ensureParsed()
	if peerPressure {
		return invPeerPress
	}
	return invOneShot
}

// SummoningPressure returns the per-member pressure prompt for the given member key.
func SummoningPressure(memberKey string) string {
	ensureParsed()
	return pressurePrompts[memberKey]
}

func readFile(name string) string {
	data, err := content.ReadFile(name)
	if err != nil {
		return ""
	}
	return string(data)
}

func parsePressurePrompts() map[string]string {
	raw := readFile("metabarons/council_summoning_prompts.md")
	if raw == "" {
		return nil
	}

	result := make(map[string]string)
	lines := strings.Split(raw, "\n")

	var currentKey string
	var sectionLines []string

	flushSection := func() {
		if currentKey != "" && len(sectionLines) > 0 {
			block := extractCodeBlock(strings.Join(sectionLines, "\n"))
			if block != "" {
				result[currentKey] = block
			}
		}
		sectionLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flushSection()
			heading := strings.TrimPrefix(trimmed, "## ")
			if key, ok := sectionToKey[heading]; ok {
				currentKey = key
			} else {
				currentKey = ""
			}
			continue
		}
		if currentKey != "" {
			sectionLines = append(sectionLines, line)
		}
	}
	flushSection()

	return result
}

func parseMembers() []MemberDef {
	var defs []MemberDef
	for _, ms := range memberSpec {
		raw := readFile(ms.File)
		card := extractCodeBlock(raw)
		def := MemberDef{
			Key:         ms.Key,
			Role:        ms.Role,
			DisplayName: MetaBaronDisplayTitle(ms.Role),
			Card:        card,
		}
		// Oda's shadow.
		if ms.Key == "oda" {
			def.Shadow = "A shadow lives in you that is not you. You are aware of it. You do not speak of it unless directly confronted."
		}
		defs = append(defs, def)
	}
	return defs
}

// extractCodeBlock returns the content between the first pair of ``` fences.
func extractCodeBlock(raw string) string {
	lines := strings.Split(raw, "\n")
	var inside bool
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inside {
				break // closing fence
			}
			inside = true
			continue
		}
		if inside {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// parseRelations parses the directional relationship matrix from links.md.
func parseRelations() map[string]map[string]string {
	raw := readFile("metabarons/relations/links.md")
	if raw == "" {
		return nil
	}

	rels := make(map[string]map[string]string)
	lines := strings.Split(raw, "\n")

	var currentSection string
	var fromKey, toKey string
	var noteLines []string

	flushNote := func() {
		if fromKey != "" && toKey != "" && len(noteLines) > 0 {
			note := strings.TrimSpace(strings.Join(noteLines, "\n"))
			if rels[fromKey] == nil {
				rels[fromKey] = make(map[string]string)
			}
			rels[fromKey][toKey] = note
		}
		fromKey = ""
		toKey = ""
		noteLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Section heading: ## OTHON VON SALZA
		if strings.HasPrefix(trimmed, "## ") {
			flushNote()
			heading := strings.TrimPrefix(trimmed, "## ")
			if key, ok := sectionToKey[heading]; ok {
				currentSection = key
			}
			continue
		}

		// Arrow: **Othon → Honorata**
		if strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, "→") {
			flushNote()
			// Extract "Othon → Honorata" from "**Othon → Honorata**"
			inner := strings.Trim(trimmed, "*")
			parts := strings.SplitN(inner, "→", 2)
			if len(parts) == 2 {
				fromName := strings.TrimSpace(parts[0])
				toName := strings.TrimSpace(parts[1])
				if fk, ok := arrowNames[fromName]; ok {
					fromKey = fk
				} else {
					fromKey = currentSection // fallback to section
				}
				if tk, ok := arrowNames[toName]; ok {
					toKey = tk
				}
			}
			continue
		}

		// Quote line: > note text
		if strings.HasPrefix(trimmed, "> ") || (strings.HasPrefix(trimmed, ">") && len(noteLines) > 0) {
			text := strings.TrimPrefix(trimmed, "> ")
			text = strings.TrimPrefix(text, ">")
			noteLines = append(noteLines, text)
			continue
		}

		// Empty line or other content — don't flush note yet, quotes may continue.
		if trimmed == "" && len(noteLines) > 0 {
			// Paragraph break in a quote block — include it.
			continue
		}

		// Non-quote, non-heading, non-arrow line — could be implementation notes etc.
		// If we have a note in progress and hit something else, flush.
		if len(noteLines) > 0 && trimmed != "" && !strings.HasPrefix(trimmed, "-") {
			flushNote()
		}
	}
	flushNote()

	// Debug: verify we got relationships.
	total := 0
	for _, m := range rels {
		total += len(m)
	}
	if total == 0 {
		fmt.Println("[metabarons] WARNING: no relationships parsed from metabarons/relations/links.md")
	}

	return rels
}
