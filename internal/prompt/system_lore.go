package prompt

import (
	"fmt"
	"strings"
)

const defaultStoryAgentSystemPrompt = "You are a Sans-Nom model agent. Follow the task prompt for your current role."

func WithMasterLoreSystemPrompt(base string, packID string, packLabel string, loreText string) string {
	loreText = strings.TrimSpace(loreText)
	if loreText == "" {
		return strings.TrimSpace(base)
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = defaultStoryAgentSystemPrompt
	}
	label := strings.TrimSpace(packLabel)
	if label == "" {
		label = strings.TrimSpace(packID)
	}
	if label == "" {
		label = "selected lorepack"
	}
	id := strings.TrimSpace(packID)
	if id == "" {
		id = "unknown"
	}
	return strings.TrimSpace(fmt.Sprintf(`%s

ACTIVE LOREPACK MASTER SUMMARY
Pack: %s (%s)
This block is stable source-lore context for all Sans-Nom story agents. Use it for lore fidelity, pressure logic, character relationships, consequence systems, and canon boundaries. The task prompt still defines your specific role and output shape.

%s

END ACTIVE LOREPACK MASTER SUMMARY`, base, label, id, loreText))
}
