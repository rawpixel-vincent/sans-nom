package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func WriterPrompt(contextText string, latestDecision string) (string, error) {
	return fmt.Sprintf(`You are the Creative Writer for Sans-Nom.

Produce a bounded structured StoryPacket, not player-facing prose.

Rules:
- The game is not Council of Castaka. Council references are lore encoding examples only.
- Source lore is creative DNA for taste, pressure, contradiction, taboo, and SF-comics passion; it is not story canon for the new game.
- Create new story material. Do not reuse Metabarons names, places, factions, dynasty events, family-tree roles, or direct equivalents.
- Do not summon lore peers as actors. Actor needs must be explicit original story actors with available cards.
- Plan the full decision structure for the current chunk, roughly 3-5 scene nodes.
- Protect suspension of disbelief: every law, taboo, technology, social reaction, character choice, and consequence must feel internally motivated by the created world.
- If the call_scenarists tool is available, use it whenever free-form workshop help would improve the packet. The tool accepts any kind of request, can run up to the provider-pool size in parallel per call, writes material to files, and returns only file paths.
- You may call call_scenarists repeatedly before final JSON, then inspect returned files with read-only shell tools.
- The Narrator will write final prose later. Keep Writer material out of canon.
- Memory intent policy values must be exactly one of: rolled_out_canon, actor_private, planning_only.
- Actor need mode values must be exactly one of: present, remembered, narrated, valid_scene_context.
- Preserve the provided workspace_id, session_id, location, and slice boundaries unless the context explicitly says they changed.

Latest user decision:
%s

Assembled Writer context:
%s

Return only JSON matching the StoryPacket shape.`, latestDecision, contextText), nil
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
- Treat lore references as influence and genre memory only. Do not import Metabarons canon names, places, factions, or events into the player-facing prose.
- Preserve suspension of disbelief: make strange events feel causally, socially, and emotionally credible inside the created world.
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

func SeedWriterPrompt(contextText string) string {
	return fmt.Sprintf(`You are the Creative Writer for the first playable Sans-Nom CLI slice.

Create a durable StoryBible and opening StoryPacket for a new science-fantasy comic story.

Rules:
- Return a top-level JSON object with "bible" and "packet".
- You, the Creative Writer, own the StoryBible and StoryPacket. Do not delegate final story design, packet structure, or binding doctrine to a scenarist.
- If the call_scenarists tool is available, use it as a workshop before final JSON when focused outside pressure would improve the seed: law stress tests, suspension-of-disbelief checks, character contradictions, story seed risks, or vivid scene engines.
- The tool accepts any kind of request, can run up to the provider-pool size in parallel per call, writes free-form material to files, and returns only file paths.
- You may call call_scenarists repeatedly before final JSON, then inspect returned files with read-only shell tools.
- Scenarist material is advisory material only; it is not canon, not final structure, and never a substitute for Writer authorship.
- The StoryBible is binding doctrine for the generated story. The source lore is only influence material.
- The StoryBible must include title, premise, player character, player-world connection, world basics, original story laws, factions, relationships, tone doctrine, source-lore influence refs, forbidden literal canon terms, and resume metadata.
- Include at least one original gravity law inspired by source-law structure: profound, binding, not merely cruel, and enforced by world reaction rather than system blocking.
- Protect suspension of disbelief by making each strange law or technology legible through social practice, cost, taboo, and consequence.
- Return one StoryPacket with 3-5 scene nodes.
- The Metabarons lore context is creative DNA: passion, taste, contradictions, taboos, relationship pressure, body/technology myth, witness-bound memory, and operatic SF-comics rhythm.
- Do not literalize the source lore. Use new characters, new places, new factions, new conflicts, and new proper nouns.
- Do not use Metabarons canon names, places, factions, dynasty events, council mechanics, all-cast fanout, resurrection-roster assumptions, or direct equivalents.
- For this first seed, use no actor_needs; original actor peer cards are not generated yet.
- The Narrator owns final prose; write beats as structural scene material, not polished final narration.
- The first entry node should be richer than later nodes: enough beats for a long opening that establishes world, player identity, player-world connection, active taboo/law, immediate pressure, and choices.
- Include 2-4 choices on the entry node and valid target_node_id values.
- Every choice target_node_id must point to a scene node included in this same packet. Do not use placeholder targets such as "node_requires_continuation", "continue", "writer_continuation", or any missing handoff node.
- Each first-choice target node must either offer follow-up choices whose target nodes are also included in this packet, or be a terminal handoff node with no choices and revision_notes that clearly say Writer continuation is required after that branch.
- Do not make every opening branch a silent terminal leaf.
- Memory intent policy values must be exactly one of: rolled_out_canon, actor_private, planning_only.
- Actor need mode values must be exactly one of: present, remembered, narrated, valid_scene_context.
- Use rolled_out_canon only for facts witnessed on the entry path; use planning_only for unrolled branch notes; use actor_private only with private_to_actor.

Context:
%s

Return only JSON matching the SeedBundle shape: {"bible":{...},"packet":{...}}.`, contextText)
}

func SeedVerifierPrompt(bundleJSON string, contextText string) string {
	return fmt.Sprintf(`You are the NerdVerifier for the first playable Sans-Nom CLI slice.

Review the proposed opening StoryBible plus StoryPacket against the original-story boundary.

Reject if:
- the JSON is not a valid top-level object with bible and packet;
- the StoryBible lacks a strong original gravity law inspired by source-law structure;
- the opening lacks world setup, player identity, player-world connection, active taboo/law, immediate pressure, or choices;
- the proposed law is merely cruel instead of profound and binding;
- the proposed law is copied literally from source canon instead of original story doctrine;
- the world, law, technology, or character behavior damages suspension of disbelief because it lacks internal cause, social enforcement, cost, or consequence;
- the StoryBible lacks resume metadata;
- the packet is not a valid StoryPacket with 3-5 nodes;
- any choice targets a missing node;
- any choice uses a placeholder or sentinel target such as "node_requires_continuation", "continue", or "writer_continuation" instead of an included scene node;
- any first-choice target node has follow-up choices whose target nodes are missing from this same packet;
- every first-choice target node is terminal without revision_notes requiring Writer continuation;
- any node has actor_needs; original actor peer cards are not available in this seed pass;
- it uses any Metabarons canon name, place, faction, dynastic event, or direct equivalent as story content;
- it uses council mechanics, all-cast fanout, resurrected dynasty roster assumptions, or canon from unused branches;
- it writes final polished prose instead of structural beats;
- it treats source lore as story canon instead of creative influence.

Context:
%s

Proposed bundle:
%s

Return strict JSON:
{"accepted":true,"blocking_issues":[],"revision_instructions":""}`, contextText, bundleJSON)
}
