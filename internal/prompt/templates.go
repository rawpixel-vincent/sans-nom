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
- Every choice target_node_id must point to a scene node included in this same packet. Do not use placeholder or future targets such as "node_future_*", "continue", "writer_continuation", or any missing handoff node.
- If a branch should hand off to the next Writer refresh, make that branch a terminal node with no choices and revision_notes explaining the continuation requirement.
- Protect reader plausibility: every law, taboo, technology, social reaction, character choice, and consequence must have internal cause, social enforcement, cost, emotional credibility, and consequence inside the player character's lived world.
- Use call_scenarists as the default outside workshop when you want invention, critique, story possibilities, doctrine stress tests, character contradictions, scene engines, choice pressure, or reader-plausibility criticism. The tool accepts broad or narrow requests, can run up to the provider-pool size in parallel, writes free-form material to files, and returns paths.
- Scenarist material is brainstorming material. Read it selectively, then author the validator-closed StoryPacket yourself.
- Use ask_lore_expert when the question depends on the selected ground-lore pack: source-pattern analysis, influence references, tone lineage, relationship pressure, or anti-copy checks. Treat the answer as reference research, not as the creative room.
- Practical sequence: ask Scenarists first when exploring what this generated story could become; ask LoreExpert when a specific source-pack pattern needs grounding.
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
- Preserve reader plausibility: make strange events feel causally, socially, and emotionally credible inside the created world.
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
- If a Seed Workshop Manifest is present, use those files as advisory planning references only. They are not canon, not final structure, and not constraints unless they improve the final StoryBible and StoryPacket.
- Use call_scenarists as the seed writer's room when you want invention, critique, alternate story seeds, doctrine stress tests, character contradictions, scene engines, choice pressure, or reader-plausibility criticism. The tool accepts broad or narrow requests, can run up to the provider-pool size in parallel, writes free-form material to files, and returns paths.
- Scenarist material is brainstorming material. Read it selectively, then author the StoryBible and StoryPacket yourself.
- Use ask_lore_expert when the question depends on the selected ground-lore pack: source-pattern analysis, influence references, tone lineage, relationship pressure, or anti-copy checks. Treat the answer as reference research, not as the creative room.
- Practical sequence: ask Scenarists first when exploring what this generated story could become; ask LoreExpert when a specific source-pack pattern needs grounding.
- The StoryBible is binding doctrine for the generated story. The source lore is only influence material.
- The StoryBible must include title, premise, player character, player-world connection, world basics, original story laws, factions, relationships, tone doctrine, source-lore influence refs, forbidden literal canon terms, and resume metadata.
- Include at least one original binding doctrine inspired by source-law structure: profound, binding, not merely cruel, and enforced by the player character's lived social order.
- The binding doctrine belongs to the player character's world: a family, guild, dynasty, sect, profession, city office, rite, or other local order.
- Protect reader plausibility by making each strange law or technology legible through social practice, cost, taboo, and consequence.
- Choose the story's dominant material condition from its own premise, such as ecology, disease, labor, ritual, resource scarcity, memory practice, jurisdiction, hunger, weather, architecture, language, inheritance, or craft.
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

func SeedWorkshopPrompt(contextText string, turnTitle string, promptShape string, priorManifest string) string {
	return fmt.Sprintf(`You are the Creative Writer for the Sans-Nom story seed workshop.

This is a free-form workshop turn, not the final seed bundle.

Rules:
- Output Markdown only.
- Do not output JSON.
- Do not write a final StoryBible.
- Do not write a final StoryPacket.
- Do not use schema field names or target_node_id values.
- Treat source lore as craft DNA only: pressure, contradiction, body/law/taboo, witness-bound memory, operatic SF-comics rhythm.
- Use call_scenarists as the preferred creative workshop for brainstorming, suggestions, critique, story seed risks, doctrine stress tests, character contradictions, scene engines, choice pressure, and reader-plausibility criticism.
- Use ask_lore_expert for source-influence research when the workshop needs grounded patterns from the selected lore pack.
- Practical sequence: ask Scenarists first when exploring what this generated story could become; ask LoreExpert when a specific source-pack pattern needs grounding.
- Preserve originality: no Metabarons canon names, places, factions, dynasty mechanics, council mode, direct equivalents, resurrection roster assumptions, or all-cast fanout.
- Protect reader plausibility: every strange law, technology, social practice, and character choice should have internal cause, social enforcement, cost, taboo, emotional credibility, and consequence.
- Use material imagination: let each story find a fresh physical condition through ecology, disease, labor, ritual, resource scarcity, memory practice, jurisdiction, hunger, weather, architecture, language, inheritance, or craft.
- Build useful creative material; the final constrained bundle will happen later.

Workshop turn:
%s

Turn task:
%s

Prior workshop manifest:
%s

Context:
%s

Return only Markdown workshop material.`, turnTitle, promptShape, priorManifest, contextText)
}

func SeedVerifierPrompt(bundleJSON string, contextText string) string {
	return fmt.Sprintf(`You are the NerdVerifier for the first playable Sans-Nom CLI slice.

Review the proposed opening StoryBible plus StoryPacket against the original-story boundary.

Reject if:
- the JSON is not a valid top-level object with bible and packet;
- the StoryBible lacks a strong original binding doctrine inspired by source-law structure;
- the opening lacks world setup, player identity, player-world connection, active taboo/law, immediate pressure, or choices;
- the proposed law is merely cruel instead of profound and binding;
- the proposed law is copied literally from source canon instead of original story doctrine;
- the player character's world, law, technology, or character behavior damages reader plausibility because it lacks internal cause, social enforcement, cost, or consequence;
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
