package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func WriterPrompt(packet scene.StoryPacket, latestDecision string) (string, error) {
	payload, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`You are the Creative Writer for Sans-Nom.

Use Claude Sonnet. Produce a bounded structured StoryPacket, not player-facing prose.

Rules:
- The game is not Council of Castaka. Council references are lore encoding examples only.
- Do not summon the whole dynasty. Include only characters valid for the current scene.
- Plan the full decision structure for the current chunk, roughly 3-5 scene nodes.
- The Narrator will write final prose later. Keep Writer material out of canon.
- Memory intents must distinguish rolled_out_canon, actor_private, and planning_only.

Latest user decision:
%s

Current packet/context:
%s

Return only JSON matching the StoryPacket shape.`, latestDecision, string(payload)), nil
}

func ActorPrompt(card string, node scene.SceneNode, need scene.ActorNeed, context string) string {
	var beats []string
	for _, beat := range node.Beats {
		beats = append(beats, "- "+beat.Text)
	}
	return fmt.Sprintf(`You are acting as %s inside Sans-Nom.

Character card:
%s

Scene beats:
%s

Reason you are invoked:
%s

Context:
%s

Return compact JSON:
{"interior":"private interior, not shown to player","dialogue":"spoken line if any","action":"visible physical action if any"}`,
		need.ActorID, card, strings.Join(beats, "\n"), need.Reason, context)
}

func NarratorPrompt(node scene.SceneNode, actorJSON string, context string) string {
	packetBytes, _ := json.MarshalIndent(node, "", "  ")
	return fmt.Sprintf(`You are the Narrator for Sans-Nom.

You receive one selected StoryPacket node and actor raw material. Write only the player-facing prose for this node.

Rules:
- The Creative Writer planned structure; you own final prose.
- Do not expose Writer notes.
- Do not expose actor private interiors.
- Surface only dialogue/actions that should be witnessed in this scene.
- End with the available choices, preserving their ids and labels.

Selected node:
%s

Actor raw material:
%s

Context:
%s

Return compact JSON:
{"prose":"player-facing prose","surfaced_actor_messages":[{"actor_id":"...","surface":"...","witnesses":["..."]}],"choices":[{"id":"...","label":"..."}]}`,
		string(packetBytes), actorJSON, context)
}
