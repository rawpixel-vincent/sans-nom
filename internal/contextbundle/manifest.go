package contextbundle

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func (a Assembler) actorManifest(packet scene.StoryPacket, node scene.SceneNode, opts Options) []ActorManifestItem {
	if a.Cards == nil {
		return nil
	}
	reasons := map[string]string{}
	explicit := map[string]bool{}
	add := func(id string, reason string, isExplicit bool) {
		id = strings.TrimSpace(id)
		if id == "" || id == opts.PlayerPeerID {
			return
		}
		if _, ok := a.Cards.LocalCard(id); !ok {
			return
		}
		if _, ok := reasons[id]; !ok {
			reasons[id] = reason
		}
		if isExplicit {
			explicit[id] = true
			reasons[id] = reason
		}
	}
	for _, need := range node.ActorNeeds {
		add(need.ActorID, "explicit actor_need", true)
		for _, target := range need.Targets {
			add(target, "actor_need target", false)
		}
	}
	for _, id := range packet.ContextSnapshot.PresentActors {
		add(id, "present in context snapshot", false)
	}

	ids := make([]string, 0, len(reasons))
	for id := range reasons {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		if explicit[ids[i]] != explicit[ids[j]] {
			return explicit[ids[i]]
		}
		return ids[i] < ids[j]
	})
	if opts.MaxActorManifestItems > 0 && len(ids) > opts.MaxActorManifestItems {
		ids = ids[:opts.MaxActorManifestItems]
	}

	out := make([]ActorManifestItem, 0, len(ids))
	for _, id := range ids {
		card, _ := a.Cards.LocalCard(id)
		out = append(out, ActorManifestItem{
			PeerID:      id,
			DisplayName: displayName(id, card),
			Role:        summarizeCard(card.Text),
			Source:      card.Source,
			Reason:      reasons[id],
			Explicit:    explicit[id],
		})
	}
	return out
}

func displayName(id string, card Card) string {
	for _, line := range strings.Split(card.Text, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"Name:", "ALIAS:"} {
			if strings.HasPrefix(line, prefix) {
				name := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if name != "" {
					return name
				}
			}
		}
	}
	base := filepath.Base(id)
	base = strings.ReplaceAll(base, "__", " ")
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return strings.TrimSpace(base)
}

func summarizeCard(text string) string {
	var firstNonName string
	for _, prefix := range []string{"ROLE:", "TRAIT:", "INSTRUCTION:", "VOICE:", "BOUNDARY:", "Name:"} {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if firstNonName == "" && !strings.HasPrefix(line, "ALIAS:") {
				firstNonName = line
			}
			if strings.HasPrefix(line, prefix) {
				return truncateSummary(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
			}
		}
	}
	return truncateSummary(firstNonName)
}

func truncateSummary(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 220 {
		return value
	}
	return strings.TrimSpace(value[:217]) + "..."
}
