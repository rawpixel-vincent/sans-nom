package contextbundle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

func DefaultOptions() Options {
	return Options{
		PlayerPeerID:           "sans-nom",
		RecentSceneCount:       3,
		NarratorLoreK:          8,
		WriterLoreK:            12,
		MaxRecentSceneChars:    600,
		MaxLoreChars:           800,
		MaxRepresentationChars: 1000,
		MaxActorCardFacts:      40,
		MaxActorTargets:        3,
		MaxActorManifestItems:  12,
		NarratorSoftCap:        12000,
		WriterSoftCap:          18000,
	}
}

func (a Assembler) AssembleNarrator(ctx context.Context, req BuildRequest) (NarratorContext, error) {
	opts := a.options()
	node, err := selectedNode(req.Packet, req.NodeID)
	if err != nil {
		return NarratorContext{}, err
	}
	if _, err := actors.NeedsForNode(node); err != nil {
		return NarratorContext{}, err
	}
	query := queryText(req.Packet, node, req.Decision)
	sceneCtx, warnings, err := a.sceneContext(ctx, req.Packet, node.ID, query, opts)
	if err != nil {
		return NarratorContext{}, err
	}
	loreHits, loreWarnings, err := a.loreHits(ctx, query, opts.NarratorLoreK, opts)
	if err != nil {
		return NarratorContext{}, err
	}
	warnings = append(warnings, loreWarnings...)
	actors, actorWarnings, err := a.actorContexts(ctx, req.Packet, node, loreHits, opts)
	if err != nil {
		return NarratorContext{}, err
	}
	warnings = append(warnings, actorWarnings...)
	reps, repWarnings := representationsFromSceneContext(sceneCtx, opts)
	warnings = append(warnings, repWarnings...)
	nc := NarratorContext{
		MaxRenderChars:   opts.NarratorSoftCap,
		StoryBible:       cloneBible(req.Bible),
		ScenarioContract: strings.TrimSpace(req.ScenarioContractText),
		SceneState:       sceneState(req.Packet),
		PlayerDecision:   req.Decision,
		SelectedNode:     publicNode(node),
		ActorContexts:    actors,
		LoreHits:         loreHits,
		RecentScenes:     trimStrings(sceneCtx.RecentScenes, opts.MaxRecentSceneChars, &warnings, "recent scene"),
		Representations:  reps,
		WitnessRules: []string{
			"Only rolled-out narrator prose, player decisions, surfaced actor speech/actions, and explicit memory intents become canon.",
			"Actor private interiors may inform narration but are not witnessed memory.",
			"Writer drafts and unused branches are planning artifacts and must not be committed to Honcho.",
			"Do not infer all-cast presence from peer cards, lore hits, or dynasty identity maps.",
		},
		Warnings: warnings,
	}
	nc.Summary = ContextBundleSummary{
		Role:         "narrator",
		PacketID:     req.Packet.ID,
		NodeID:       node.ID,
		ActorCount:   len(nc.ActorContexts),
		LoreHitCount: len(nc.LoreHits),
		Warnings:     nc.Warnings,
	}
	if opts.NarratorSoftCap > 0 && len(nc.RenderText()) > opts.NarratorSoftCap {
		nc.Warnings = appendStable(nc.Warnings, fmt.Sprintf("narrator context exceeds soft cap %d chars", opts.NarratorSoftCap))
		nc.Summary.Warnings = nc.Warnings
	}
	return nc, nil
}

func (a Assembler) AssembleWriter(ctx context.Context, req BuildRequest) (WriterContext, error) {
	opts := a.options()
	node, err := selectedNode(req.Packet, req.NodeID)
	if err != nil {
		return WriterContext{}, err
	}
	query := queryText(req.Packet, node, req.Decision)
	sceneCtx, warnings, err := a.sceneContext(ctx, req.Packet, node.ID, query, opts)
	if err != nil {
		return WriterContext{}, err
	}
	loreHits, loreWarnings, err := a.loreHits(ctx, query, opts.WriterLoreK, opts)
	if err != nil {
		return WriterContext{}, err
	}
	warnings = append(warnings, loreWarnings...)
	reps, repWarnings := representationsFromSceneContext(sceneCtx, opts)
	warnings = append(warnings, repWarnings...)
	manifest := a.actorManifest(req.Packet, node, opts)
	wc := WriterContext{
		MaxRenderChars:   opts.WriterSoftCap,
		StoryBible:       cloneBible(req.Bible),
		ScenarioContract: strings.TrimSpace(req.ScenarioContractText),
		Progression:      buildStoryProgression(req.Bible, req.Packet),
		SceneState:       sceneState(req.Packet),
		PlayerDecision:   req.Decision,
		SelectedNodeID:   node.ID,
		Packet:           req.Packet,
		RecentScenes:     trimStrings(sceneCtx.RecentScenes, opts.MaxRecentSceneChars, &warnings, "recent scene"),
		Representations:  reps,
		LoreByKind:       groupLoreByKind(loreHits),
		ActorManifest:    manifest,
		Warnings:         warnings,
	}
	wc.Summary = ContextBundleSummary{
		Role:         "writer",
		PacketID:     req.Packet.ID,
		NodeID:       node.ID,
		ActorCount:   len(manifest),
		LoreHitCount: len(loreHits),
		Warnings:     wc.Warnings,
	}
	if opts.WriterSoftCap > 0 && len(wc.RenderText()) > opts.WriterSoftCap {
		wc.Warnings = appendStable(wc.Warnings, fmt.Sprintf("writer context exceeds soft cap %d chars", opts.WriterSoftCap))
		wc.Summary.Warnings = wc.Warnings
	}
	return wc, nil
}

func (a Assembler) AssembleActorCall(ctx context.Context, req ActorCallBuildRequest) (ActorContext, string, []string, error) {
	opts := a.options()
	node := req.Node
	node.ActorNeeds = []scene.ActorNeed{req.Need}
	if err := validateSingleActorNeed(req.Need); err != nil {
		return ActorContext{}, "", nil, err
	}
	query := actorCallQueryText(req.Packet, node, req.Decision, req.Need)
	loreHits, loreWarnings, err := a.loreHits(ctx, query, opts.NarratorLoreK, opts)
	if err != nil {
		return ActorContext{}, "", nil, err
	}
	actorContexts, actorWarnings, err := a.actorContexts(ctx, req.Packet, node, loreHits, opts)
	if err != nil {
		return ActorContext{}, "", nil, err
	}
	warnings := append([]string(nil), loreWarnings...)
	warnings = append(warnings, actorWarnings...)
	if len(actorContexts) != 1 {
		return ActorContext{}, "", uniqueStrings(warnings), fmt.Errorf("actor call for %q produced %d actor contexts", req.Need.ActorID, len(actorContexts))
	}
	nc := NarratorContext{
		MaxRenderChars:   opts.NarratorSoftCap,
		StoryBible:       cloneBible(req.Bible),
		ScenarioContract: strings.TrimSpace(req.ScenarioContractText),
		SceneState:       sceneState(req.Packet),
		PlayerDecision:   req.Decision,
		SelectedNode:     publicNode(node),
		ActorContexts:    actorContexts,
		WitnessRules: []string{
			"Actor material is planning/raw performance until the Narrator surfaces dialogue or action.",
			"Private interiors are never committed to shared scene memory.",
			"Use only witnessed canon, this actor card, and provided directional representations.",
		},
		Warnings: uniqueStrings(warnings),
	}
	return actorContexts[0], nc.RenderActorText(), nc.Warnings, nil
}

func validateSingleActorNeed(need scene.ActorNeed) error {
	if strings.TrimSpace(need.ActorID) == "" {
		return errors.New("actor_id is required")
	}
	switch need.Mode {
	case scene.ActorModePresent, scene.ActorModeRemembered, scene.ActorModeNarrated, scene.ActorModeContext:
		return nil
	default:
		return fmt.Errorf("actor %q has invalid mode %q", need.ActorID, need.Mode)
	}
}

func actorCallQueryText(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision, need scene.ActorNeed) string {
	parts := []string{queryText(packet, node, decision), need.ActorID, need.Reason}
	parts = append(parts, need.Targets...)
	return strings.Join(nonEmptyStrings(parts), "\n")
}

func cloneBible(bible *story.StoryBible) *story.StoryBible {
	if bible == nil {
		return nil
	}
	cloned := *bible
	cloned.WorldBasics = append([]string(nil), bible.WorldBasics...)
	cloned.LorePrinciples = append([]story.OriginalLaw(nil), bible.LorePrinciples...)
	cloned.OriginalLaws = append([]story.OriginalLaw(nil), bible.OriginalLaws...)
	cloned.Factions = append([]story.Faction(nil), bible.Factions...)
	cloned.CoreRelationships = append([]string(nil), bible.CoreRelationships...)
	cloned.ToneDoctrine = append([]string(nil), bible.ToneDoctrine...)
	cloned.SourceLoreInfluenceRefs = append([]string(nil), bible.SourceLoreInfluenceRefs...)
	cloned.ForbiddenLiteralCanonTerms = append([]string(nil), bible.ForbiddenLiteralCanonTerms...)
	cloned.FirstTurnExposition = append([]string(nil), bible.FirstTurnExposition...)
	cloned.StoryArc = append([]string(nil), bible.StoryArc...)
	cloned.SpentMotifs = append([]string(nil), bible.SpentMotifs...)
	cloned.OpenThreads = append([]string(nil), bible.OpenThreads...)
	if bible.Metadata != nil {
		cloned.Metadata = map[string]string{}
		for key, value := range bible.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}

func (a Assembler) options() Options {
	opts := DefaultOptions()
	if a.Options.PlayerPeerID != "" {
		opts.PlayerPeerID = a.Options.PlayerPeerID
	}
	copyPositiveInt(&opts.RecentSceneCount, a.Options.RecentSceneCount)
	copyPositiveInt(&opts.NarratorLoreK, a.Options.NarratorLoreK)
	copyPositiveInt(&opts.WriterLoreK, a.Options.WriterLoreK)
	copyPositiveInt(&opts.MaxRecentSceneChars, a.Options.MaxRecentSceneChars)
	copyPositiveInt(&opts.MaxLoreChars, a.Options.MaxLoreChars)
	copyPositiveInt(&opts.MaxRepresentationChars, a.Options.MaxRepresentationChars)
	copyPositiveInt(&opts.MaxActorCardFacts, a.Options.MaxActorCardFacts)
	copyPositiveInt(&opts.MaxActorTargets, a.Options.MaxActorTargets)
	copyPositiveInt(&opts.MaxActorManifestItems, a.Options.MaxActorManifestItems)
	copyPositiveInt(&opts.NarratorSoftCap, a.Options.NarratorSoftCap)
	copyPositiveInt(&opts.WriterSoftCap, a.Options.WriterSoftCap)
	opts.RequireBridge = a.Options.RequireBridge
	opts.RequireLore = a.Options.RequireLore
	opts.SkipMissingActorCards = a.Options.SkipMissingActorCards
	return opts
}

func copyPositiveInt(dst *int, value int) {
	if value > 0 {
		*dst = value
	}
}

func selectedNode(packet scene.StoryPacket, nodeID string) (scene.SceneNode, error) {
	if nodeID == "" {
		return packet.EntryNode()
	}
	return packet.Node(nodeID)
}

func sceneState(packet scene.StoryPacket) SceneState {
	return SceneState{
		WorkspaceID:   packet.ContextSnapshot.WorkspaceID,
		SessionID:     packet.ContextSnapshot.SessionID,
		Location:      packet.ContextSnapshot.Location,
		Time:          packet.ContextSnapshot.Time,
		PresentActors: sortedCopy(packet.ContextSnapshot.PresentActors),
	}
}

func publicNode(node scene.SceneNode) PublicSceneNode {
	return PublicSceneNode{
		ID:            node.ID,
		Title:         node.Title,
		Beats:         append([]scene.Beat(nil), node.Beats...),
		Gates:         append([]scene.Gate(nil), node.Gates...),
		Choices:       append([]scene.Choice(nil), node.Choices...),
		ActorNeeds:    append([]scene.ActorNeed(nil), node.ActorNeeds...),
		MemoryIntents: append([]scene.MemoryIntent(nil), node.MemoryIntents...),
	}
}

func (a Assembler) sceneContext(
	ctx context.Context,
	packet scene.StoryPacket,
	nodeID string,
	query string,
	opts Options,
) (bridgeclient.SceneContextResponse, []string, error) {
	if a.Bridge == nil {
		return fallbackSceneContext(packet, opts), []string{"bridge unavailable; using StoryPacket context snapshot"}, nil
	}
	sceneSessionID := scene.SceneSessionID(packet.ContextSnapshot.SessionID, packet.ID, nodeID)
	resp, err := a.Bridge.SceneContext(ctx, bridgeclient.SceneContextRequest{
		WorkspaceID:      packet.ContextSnapshot.WorkspaceID,
		SessionID:        sceneSessionID,
		PlayerPeerID:     opts.PlayerPeerID,
		RecentSceneCount: opts.RecentSceneCount,
		Representations:  []string{"narrator:" + opts.PlayerPeerID},
		SearchQuery:      query,
	})
	if err != nil {
		if opts.RequireBridge {
			return bridgeclient.SceneContextResponse{}, nil, err
		}
		return fallbackSceneContext(packet, opts), []string{"bridge scene context failed; using StoryPacket context snapshot"}, nil
	}
	resp.RecentScenes = mergeFallbackRecentScenes(resp.RecentScenes, packet.ContextSnapshot.LatestNarratorProse, opts)
	return resp, nil, nil
}

func fallbackSceneContext(packet scene.StoryPacket, opts Options) bridgeclient.SceneContextResponse {
	var recent []string
	if packet.ContextSnapshot.LatestNarratorProse != "" {
		recent = append(recent, packet.ContextSnapshot.LatestNarratorProse)
	}
	return bridgeclient.SceneContextResponse{
		WorkspaceID:     packet.ContextSnapshot.WorkspaceID,
		SessionID:       packet.ContextSnapshot.SessionID,
		RecentScenes:    trimToCount(recent, opts.RecentSceneCount),
		Representations: copyStringMap(packet.ContextSnapshot.Representations),
	}
}

func mergeFallbackRecentScenes(recent []string, latest string, opts Options) []string {
	latest = strings.TrimSpace(latest)
	if latest == "" {
		return trimToCount(recent, opts.RecentSceneCount)
	}
	for _, scene := range recent {
		if strings.TrimSpace(scene) == latest {
			return trimToCount(recent, opts.RecentSceneCount)
		}
	}
	merged := append([]string{latest}, recent...)
	return trimToCount(merged, opts.RecentSceneCount)
}

func (a Assembler) loreHits(ctx context.Context, query string, k int, opts Options) ([]LoreHit, []string, error) {
	if a.Lore == nil {
		if opts.RequireLore {
			return nil, nil, errors.New("lore retriever is required")
		}
		return nil, []string{"lore retriever unavailable; no runtime lore hits included"}, nil
	}
	snippets, err := a.Lore.Search(ctx, query, k)
	if err != nil {
		if opts.RequireLore {
			return nil, nil, err
		}
		return nil, []string{"lore retrieval failed; no runtime lore hits included"}, nil
	}
	hits := make([]LoreHit, 0, len(snippets))
	warnings := []string{}
	seen := map[string]struct{}{}
	for _, snippet := range snippets {
		if _, ok := seen[snippet.ID]; ok {
			continue
		}
		seen[snippet.ID] = struct{}{}
		text := truncate(snippet.Text, opts.MaxLoreChars, &warnings, "lore hit "+snippet.ID)
		hits = append(hits, LoreHit{
			ID:          snippet.ID,
			Text:        text,
			Source:      snippet.Source,
			Score:       snippet.Score,
			Kind:        snippet.Kind,
			PageID:      snippet.PageID,
			PeerID:      snippet.PeerID,
			CharacterID: snippet.CharacterID,
			Volume:      snippet.Volume,
		})
	}
	sortLore(hits)
	return hits, warnings, nil
}

func (a Assembler) actorContexts(
	ctx context.Context,
	packet scene.StoryPacket,
	node scene.SceneNode,
	loreHits []LoreHit,
	opts Options,
) ([]ActorContext, []string, error) {
	needs, err := actors.NeedsForNode(node)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(needs, func(i, j int) bool {
		if needs[i].ActorID == needs[j].ActorID {
			return needs[i].Mode < needs[j].Mode
		}
		return needs[i].ActorID < needs[j].ActorID
	})
	var out []ActorContext
	var warnings []string
	for _, need := range needs {
		card, cardWarnings, err := a.actorCard(ctx, packet.ContextSnapshot.WorkspaceID, need.ActorID, opts)
		if err != nil {
			if opts.SkipMissingActorCards {
				warnings = append(warnings, fmt.Sprintf("actor %q skipped: no Honcho or local card", need.ActorID))
				continue
			}
			return nil, nil, err
		}
		warnings = append(warnings, cardWarnings...)
		reps, repWarnings, err := a.actorRepresentations(ctx, packet, need, opts)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, repWarnings...)
		actorWarnings := append([]string(nil), cardWarnings...)
		actorWarnings = append(actorWarnings, repWarnings...)
		actorLore := loreForActor(loreHits, need.ActorID)
		out = append(out, ActorContext{
			ActorID:         need.ActorID,
			Mode:            need.Mode,
			Reason:          need.Reason,
			Targets:         trimToCount(sortedCopy(need.Targets), opts.MaxActorTargets),
			Card:            card.Text,
			CardSource:      card.Source,
			Representations: reps,
			LoreHits:        actorLore,
			Warnings:        uniqueStrings(actorWarnings),
		})
	}
	return out, uniqueStrings(warnings), nil
}

func (a Assembler) actorCard(ctx context.Context, workspaceID, actorID string, opts Options) (Card, []string, error) {
	if a.Bridge != nil {
		facts, err := a.Bridge.GetCard(ctx, workspaceID, actorID, "")
		if err != nil {
			if opts.RequireBridge {
				return Card{}, nil, err
			}
		} else if len(trimFacts(facts, opts.MaxActorCardFacts)) > 0 {
			return Card{
				ActorID: actorID,
				Text:    strings.Join(trimFacts(facts, opts.MaxActorCardFacts), "\n"),
				Source:  "honcho:" + actorID,
			}, nil, nil
		}
	}
	if a.Cards != nil {
		if card, ok := a.Cards.LocalCard(actorID); ok {
			lines := trimFacts(strings.Split(card.Text, "\n"), opts.MaxActorCardFacts)
			card.Text = strings.Join(lines, "\n")
			warnings := []string{"fallback card source for " + actorID + ": " + card.Source}
			if a.Bridge != nil && len(lines) > 0 {
				if seeded, err := a.Bridge.PatchCard(ctx, workspaceID, actorID, bridgeclient.PeerCardPatch{Facts: lines, Replace: true}); err != nil {
					if opts.RequireBridge {
						return Card{}, nil, err
					}
					warnings = append(warnings, "Honcho seed card failed for "+actorID+": "+err.Error())
				} else if len(trimFacts(seeded, opts.MaxActorCardFacts)) > 0 {
					return Card{
						ActorID: actorID,
						Text:    strings.Join(trimFacts(seeded, opts.MaxActorCardFacts), "\n"),
						Source:  "honcho:" + actorID + " seeded from " + card.Source,
					}, warnings, nil
				}
			}
			return card, warnings, nil
		}
	}
	return Card{}, nil, fmt.Errorf("actor %q has no Honcho or local card", actorID)
}

func (a Assembler) actorRepresentations(
	ctx context.Context,
	packet scene.StoryPacket,
	need scene.ActorNeed,
	opts Options,
) ([]Representation, []string, error) {
	targets := actorRepresentationTargets(need.Targets, opts)
	var out []Representation
	var warnings []string
	for _, target := range targets {
		rep, err := a.representation(ctx, packet, need.ActorID, target, opts)
		if err != nil {
			return nil, nil, err
		}
		if rep.Text == "" {
			warnings = append(warnings, fmt.Sprintf("representation unavailable: %s -> %s", need.ActorID, target))
			continue
		}
		rep.Text = truncate(rep.Text, opts.MaxRepresentationChars, &warnings, "representation "+need.ActorID+" -> "+target)
		out = append(out, rep)
	}
	return out, uniqueStrings(warnings), nil
}

func actorRepresentationTargets(needTargets []string, opts Options) []string {
	targets := []string{opts.PlayerPeerID}
	seen := map[string]struct{}{opts.PlayerPeerID: {}}
	count := 0
	for _, target := range sortedCopy(needTargets) {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		if opts.MaxActorTargets > 0 && count >= opts.MaxActorTargets {
			break
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
		count++
	}
	return targets
}

func (a Assembler) representation(
	ctx context.Context,
	packet scene.StoryPacket,
	observer string,
	observed string,
	opts Options,
) (Representation, error) {
	if a.Bridge != nil {
		resp, err := a.Bridge.Representation(ctx, bridgeclient.RepresentationRequest{
			WorkspaceID: packet.ContextSnapshot.WorkspaceID,
			Observer:    observer,
			Observed:    observed,
			SearchQuery: queryText(packet, scene.SceneNode{}, scene.PlayerDecision{}),
			TopK:        10,
		})
		if err != nil {
			if opts.RequireBridge {
				return Representation{}, err
			}
		} else {
			return Representation{Observer: observer, Observed: observed, Text: resp.Representation}, nil
		}
	}
	key := observer + ":" + observed
	return Representation{Observer: observer, Observed: observed, Text: packet.ContextSnapshot.Representations[key]}, nil
}

func representationsFromSceneContext(resp bridgeclient.SceneContextResponse, opts Options) ([]Representation, []string) {
	var out []Representation
	var warnings []string
	for key, value := range resp.Representations {
		if strings.TrimSpace(value) == "" {
			continue
		}
		observer, observed := splitRepresentationKey(key, opts.PlayerPeerID)
		out = append(out, Representation{
			Observer: observer,
			Observed: observed,
			Text:     truncate(value, opts.MaxRepresentationChars, &warnings, "scene representation "+key),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Observer == out[j].Observer {
			return out[i].Observed < out[j].Observed
		}
		return out[i].Observer < out[j].Observer
	})
	return out, uniqueStrings(warnings)
}

func splitRepresentationKey(key string, defaultObserved string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "narrator", defaultObserved
}

func groupLoreByKind(hits []LoreHit) map[string][]LoreHit {
	out := map[string][]LoreHit{}
	for _, hit := range hits {
		kind := hit.Kind
		if kind == "" {
			kind = "unknown"
		}
		out[kind] = append(out[kind], hit)
	}
	for kind := range out {
		sortLore(out[kind])
	}
	return out
}

func loreForActor(hits []LoreHit, actorID string) []LoreHit {
	var out []LoreHit
	for _, hit := range hits {
		if hit.PeerID == actorID || hit.CharacterID == actorID {
			out = append(out, hit)
		}
	}
	sortLore(out)
	return out
}
