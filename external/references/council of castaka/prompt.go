package council

import (
	"fmt"
	"strings"
)

// assembleOneShot builds the full prompt for a one-shot council invocation.
func assembleOneShot(member MemberDef, all []MemberDef, matter string, steelheadPhase string) string {
	var b strings.Builder

	// Character card.
	b.WriteString(member.Card)
	b.WriteString("\n")

	// Special cases.
	if member.Key == "steelhead" {
		fmt.Fprintf(&b, "\n[You are being invoked in your %s phase.]\n", steelheadPhase)
	}
	if member.Shadow != "" {
		fmt.Fprintf(&b, "\n%s\n", member.Shadow)
	}

	// Invocation preamble.
	template := InvocationTemplate(false)
	// Replace placeholders.
	template = strings.Replace(template, "[CHARACTER SUMMONING PRESSURE PROMPT]", SummoningPressure(member.Key), 1)
	b.WriteString("\n")
	b.WriteString(template)

	// Roster: other council members with relationship notes.
	b.WriteString("\n\nOTHER COUNCIL MEMBERS PRESENT:\n")
	for _, other := range all {
		if other.Key == member.Key {
			continue
		}
		note := RelationshipNote(member.Key, other.Key)
		fmt.Fprintf(&b, "- %s\n", other.DisplayName)
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}

	// The matter.
	b.WriteString("\n---\n\n")
	b.WriteString(matter)
	b.WriteString("\n")

	return b.String()
}

// assembleRound1 builds the prompt for peer pressure round 1.
// The member is asked to output a private message (optional) and a public statement.
func assembleRound1(member MemberDef, all []MemberDef, matter string, steelheadPhase string) string {
	var b strings.Builder

	// Character card.
	b.WriteString(member.Card)
	b.WriteString("\n")

	if member.Key == "steelhead" {
		fmt.Fprintf(&b, "\n[You are being invoked in your %s phase.]\n", steelheadPhase)
	}
	if member.Shadow != "" {
		fmt.Fprintf(&b, "\n%s\n", member.Shadow)
	}

	// Peer pressure invocation.
	template := InvocationTemplate(true)
	template = strings.Replace(template, "[CHARACTER SUMMONING PRESSURE PROMPT]", SummoningPressure(member.Key), 1)
	b.WriteString("\n")
	b.WriteString(template)

	// Roster.
	b.WriteString("\n\nOTHER COUNCIL MEMBERS PRESENT:\n")
	for _, other := range all {
		if other.Key == member.Key {
			continue
		}
		note := RelationshipNote(member.Key, other.Key)
		fmt.Fprintf(&b, "- %s\n", other.DisplayName)
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}
	allKeys := make([]string, len(all))
	for _, m := range all {
		allKeys = append(allKeys, m.Key)
	}
	if member.Key == "sans-nom" && !strings.Contains(strings.Join(allKeys, " "), "aghora") {
		note := RelationshipNote("sans-nom", "aghora")
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}

	// The matter.
	b.WriteString("\n---\n\n")
	b.WriteString(matter)

	// Structured output instructions.
	b.WriteString("\n\n---\n\n")
	b.WriteString("Format your response EXACTLY as follows:\n\n")
	b.WriteString("PRIVATE TO: [name of the council member you wish to address]\n")
	b.WriteString("[your private message]\n\n")
	b.WriteString("===END PRIVATE===\n\n")
	b.WriteString("PUBLIC STATEMENT:\n")
	b.WriteString("[your public statement on the matter]\n")
	b.WriteString("\nIf you do not wish to send a private message, write:\n\n")
	b.WriteString("NO PRIVATE MESSAGE\n\n")
	b.WriteString("===END PRIVATE===\n\n")
	b.WriteString("PUBLIC STATEMENT:\n")
	b.WriteString("[your public statement]\n")

	return b.String()
}

// assembleRound2 builds the prompt for peer pressure round 2.
// The member receives private messages and gives their final public statement.
func assembleRound2(member MemberDef, all []MemberDef, matter string, steelheadPhase string, sentTo string, sentMsg string, sentReply string, received []PrivateExchange) string {
	var b strings.Builder

	// Character card (full context for fresh call).
	b.WriteString(member.Card)
	b.WriteString("\n")

	if member.Key == "steelhead" {
		fmt.Fprintf(&b, "\n[You are being invoked in your %s phase.]\n", steelheadPhase)
	}
	if member.Shadow != "" {
		fmt.Fprintf(&b, "\n%s\n", member.Shadow)
	}

	// Full peer pressure invocation.
	template := InvocationTemplate(true)
	template = strings.Replace(template, "[CHARACTER SUMMONING PRESSURE PROMPT]", SummoningPressure(member.Key), 1)
	b.WriteString("\n")
	b.WriteString(template)

	// Roster with relationship notes.
	b.WriteString("\n\nOTHER COUNCIL MEMBERS PRESENT:\n")
	for _, other := range all {
		if other.Key == member.Key {
			continue
		}
		note := RelationshipNote(member.Key, other.Key)
		fmt.Fprintf(&b, "- %s\n", other.DisplayName)
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}
	allKeys := make([]string, len(all))
	for _, m := range all {
		allKeys = append(allKeys, m.Key)
	}
	if member.Key == "sans-nom" && !strings.Contains(strings.Join(allKeys, " "), "aghora") {
		note := RelationshipNote("sans-nom", "aghora")
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}

	// Private exchange context.
	b.WriteString("\n--- PRIVATE EXCHANGES ---\n\n")

	if sentTo != "" {
		fmt.Fprintf(&b, "You sent a private message to %s:\n> %s\n\n", sentTo, sentMsg)
		fmt.Fprintf(&b, "Their response: %s\n\n", sentReply)
	}

	if len(received) > 0 {
		b.WriteString("Private messages you received:\n")
		for _, ex := range received {
			fmt.Fprintf(&b, "From %s:\n> %s\n\n", ex.From, ex.Message)
		}
	}

	if sentTo == "" && len(received) == 0 {
		b.WriteString("No private messages were exchanged with you.\n")
	}

	// The matter.
	b.WriteString("\n--- MATTER BEFORE THE COUNCIL ---\n\n")
	b.WriteString(matter)
	b.WriteString("\n\nGive your final public statement.\n")

	return b.String()
}

// assembleScrutiny builds the prompt for the re-evaluation round.
// Each member receives all other members' final statements and speaks last.
func assembleScrutiny(member MemberDef, all []MemberDef, matter string, steelheadPhase string, others []MemberStatement) string {
	var b strings.Builder

	// Character card.
	b.WriteString(member.Card)
	b.WriteString("\n")

	if member.Key == "steelhead" {
		fmt.Fprintf(&b, "\n[You are being invoked in your %s phase.]\n", steelheadPhase)
	}
	if member.Shadow != "" {
		fmt.Fprintf(&b, "\n%s\n", member.Shadow)
	}

	// Pressure prompt + council framing (full invocation template).
	template := InvocationTemplate(true)
	template = strings.Replace(template, "[CHARACTER SUMMONING PRESSURE PROMPT]", SummoningPressure(member.Key), 1)
	b.WriteString("\n")
	b.WriteString(template)

	// Roster with relationship notes.
	b.WriteString("\n\nOTHER COUNCIL MEMBERS PRESENT:\n")
	for _, other := range all {
		if other.Key == member.Key {
			continue
		}
		note := RelationshipNote(member.Key, other.Key)
		fmt.Fprintf(&b, "- %s\n", other.DisplayName)
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}

	allKeys := make([]string, len(all))
	for _, m := range all {
		allKeys = append(allKeys, m.Key)
	}
	if member.Key == "sans-nom" && !strings.Contains(strings.Join(allKeys, " "), "aghora") {
		note := RelationshipNote("sans-nom", "aghora")
		if note != "" {
			fmt.Fprintf(&b, "  (%s)\n", note)
		}
	}

	// The matter.
	b.WriteString("\n---\n\n")
	b.WriteString(matter)

	// All other members' statements.
	b.WriteString("\n\n---\n\n")
	b.WriteString("The other council members have spoken. Their positions are recorded below.\n")
	b.WriteString("You are the last to speak. You have heard them all.\n\n")
	for _, s := range others {
		fmt.Fprintf(&b, "**%s:**\n%s\n\n", s.Name, s.Statement)
	}

	b.WriteString("---\n\n")
	b.WriteString("You have heard every position. Now give your final word.\n")
	b.WriteString("Do not summarize the others. Do not soften your own position to accommodate them.\n")
	b.WriteString("Speak as the last voice in the room — knowing everything, committing to what you actually believe.\n")

	return b.String()
}

// parseRound1Output extracts the private message target, message, and public statement
// from a round 1 response.
func parseRound1Output(output string) (targetKey string, privateMsg string, publicStatement string) {
	// Look for "PRIVATE TO: <name>" and "===END PRIVATE===" markers.
	lines := strings.Split(output, "\n")

	var (
		inPrivate bool
		privateTo string
		privLines []string
		pubLines  []string
		inPublic  bool
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToUpper(trimmed), "PRIVATE TO:") {
			inPrivate = true
			inPublic = false
			privateTo = strings.TrimSpace(trimmed[len("PRIVATE TO:"):])
			continue
		}

		if strings.ToUpper(trimmed) == "NO PRIVATE MESSAGE" {
			inPrivate = false
			continue
		}

		if strings.Contains(strings.ToUpper(trimmed), "===END PRIVATE===") {
			inPrivate = false
			continue
		}

		if strings.HasPrefix(strings.ToUpper(trimmed), "PUBLIC STATEMENT:") {
			inPublic = true
			inPrivate = false
			// Check if there's content after the colon on the same line.
			rest := strings.TrimSpace(line[strings.Index(strings.ToUpper(line), "PUBLIC STATEMENT:")+len("PUBLIC STATEMENT:"):])
			if rest != "" {
				pubLines = append(pubLines, rest)
			}
			continue
		}

		if inPrivate {
			privLines = append(privLines, line)
		} else if inPublic {
			pubLines = append(pubLines, line)
		}
	}

	// Resolve target name to key.
	if privateTo != "" {
		privateTo = strings.Trim(privateTo, "[]")
		for name, key := range ArrowNameToKey() {
			if strings.EqualFold(privateTo, name) || strings.Contains(strings.ToLower(privateTo), strings.ToLower(name)) {
				targetKey = key
				break
			}
		}
		// Also try matching against display names.
		if targetKey == "" {
			for _, m := range AllMembers() {
				if strings.Contains(strings.ToLower(privateTo), strings.ToLower(m.Key)) ||
					strings.Contains(strings.ToLower(m.DisplayName), strings.ToLower(privateTo)) {
					targetKey = m.Key
					break
				}
			}
		}
	}

	privateMsg = strings.TrimSpace(strings.Join(privLines, "\n"))
	publicStatement = strings.TrimSpace(strings.Join(pubLines, "\n"))

	// If no structured output was found, treat entire output as public statement.
	if publicStatement == "" && privateMsg == "" {
		publicStatement = strings.TrimSpace(output)
	}

	return targetKey, privateMsg, publicStatement
}
