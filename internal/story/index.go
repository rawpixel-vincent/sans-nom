package story

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type IndexDocument struct {
	DocID        string         `json:"doc_id"`
	PointID      string         `json:"point_id"`
	Kind         string         `json:"kind"`
	SourcePath   string         `json:"source_path"`
	SourceSHA256 string         `json:"source_sha256"`
	TextSHA256   string         `json:"text_sha256"`
	Text         string         `json:"text"`
	StoryID      string         `json:"story_id,omitempty"`
	PacketID     string         `json:"packet_id,omitempty"`
	NodeID       string         `json:"node_id,omitempty"`
	Turn         int            `json:"turn,omitempty"`
	Heading      string         `json:"heading,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

type IndexManifest struct {
	GeneratedAt    string              `json:"generated_at"`
	Collection     string              `json:"collection"`
	EmbeddingModel string              `json:"embedding_model"`
	DocumentCount  int                 `json:"document_count"`
	CountsByKind   map[string]int      `json:"counts_by_kind"`
	Sources        []IndexManifestItem `json:"sources"`
	Documents      []IndexManifestItem `json:"documents"`
}

type IndexManifestItem struct {
	DocID      string `json:"doc_id,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Path       string `json:"path,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	TextSHA256 string `json:"text_sha256,omitempty"`
}

func BuildIndexDocuments(bible StoryBible, packet scene.StoryPacket, turns []TurnRecord) []IndexDocument {
	var docs []IndexDocument
	source := "bible.json"
	docs = append(docs, newIndexDocument(
		fmt.Sprintf("story_bible:%s", bible.StoryID),
		"story_bible",
		source,
		storyBibleIndexText(bible),
		bible.StoryID,
		"",
		"",
		0,
		"Story Bible",
		map[string]any{"title": bible.Title},
	))
	for _, law := range bible.BindingLorePrinciples() {
		docs = append(docs, newIndexDocument(
			fmt.Sprintf("story_lore_principle:%s:%s", bible.StoryID, law.ID),
			"story_lore_principle",
			source,
			lorePrincipleIndexText(bible, law),
			bible.StoryID,
			"",
			"",
			0,
			law.Name,
			map[string]any{"law_id": law.ID, "law_name": law.Name},
		))
	}
	for _, card := range storyPeerCardDocuments(bible) {
		docs = append(docs, newIndexDocument(
			fmt.Sprintf("story_peer_card:%s:%s", bible.StoryID, card.peerID),
			"story_peer_card",
			source,
			card.text,
			bible.StoryID,
			"",
			"",
			0,
			card.peerID,
			map[string]any{"peer_id": card.peerID},
		))
	}
	if packet.ID != "" {
		packetSource := filepath.ToSlash(filepath.Join("packets", packet.ID+".json"))
		docs = append(docs, newIndexDocument(
			fmt.Sprintf("story_packet:%s:%s", bible.StoryID, packet.ID),
			"story_packet",
			packetSource,
			packetIndexText(packet),
			bible.StoryID,
			packet.ID,
			"",
			0,
			"Current StoryPacket",
			map[string]any{"entry_node_id": packet.EntryNodeID},
		))
		for _, node := range packet.Nodes {
			docs = append(docs, newIndexDocument(
				fmt.Sprintf("story_packet_node:%s:%s:%s", bible.StoryID, packet.ID, node.ID),
				"story_packet_node",
				packetSource,
				nodeIndexText(packet, node),
				bible.StoryID,
				packet.ID,
				node.ID,
				0,
				node.Title,
				map[string]any{"choice_count": len(node.Choices)},
			))
		}
	}
	for _, turn := range turns {
		docs = append(docs, newIndexDocument(
			fmt.Sprintf("story_turn:%s:%06d", bible.StoryID, turn.Turn),
			"story_turn",
			fmt.Sprintf("narration/turn-%06d.md", turn.Turn),
			turnIndexText(turn),
			bible.StoryID,
			turn.PacketID,
			turn.NodeID,
			turn.Turn,
			fmt.Sprintf("Turn %d", turn.Turn),
			map[string]any{
				"choice_id": turn.Decision.ChoiceID,
				"free_text": turn.Decision.FreeText,
				"committed": turn.Committed,
			},
		))
	}
	docs = compactDocs(docs)
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].DocID < docs[j].DocID })
	return docs
}

type storyPeerCardDoc struct {
	peerID string
	text   string
}

func storyPeerCardDocuments(bible StoryBible) []storyPeerCardDoc {
	laws := make([]string, 0, len(bible.BindingLorePrinciples()))
	for _, law := range bible.BindingLorePrinciples() {
		laws = append(laws, "INSTRUCTION: Enforce binding lore principle "+law.Name+": "+law.Enforcement)
	}
	narrator := []string{
		"Name: narrator",
		"ROLE: Player-facing narrator for " + bible.Title + ".",
		"INSTRUCTION: Render selected nodes into witnessed prose grounded in the Story Bible.",
		"BOUNDARY: Lorepack references support the active scenario contract; unused lore is not automatic scene canon.",
	}
	narrator = append(narrator, laws...)
	writer := []string{
		"Name: creative-writer",
		"ROLE: Background planner for bounded StoryPacket revisions in " + bible.Title + ".",
		"INSTRUCTION: Use the Story Bible and lorepack scenario contract as the active story contract.",
		"BOUNDARY: Writer drafts and unused branches are planning artifacts.",
	}
	writer = append(writer, laws...)
	player := []string{
		"Name: " + bible.PlayerCharacter.Name,
		"ROLE: Player character peer " + bible.PlayerCharacter.PeerID + ".",
		"TRAIT: " + bible.PlayerCharacter.Description,
		"TRAIT: Desire: " + bible.PlayerCharacter.Desire,
		"TRAIT: Wound: " + bible.PlayerCharacter.Wound,
		"BOUNDARY: " + bible.PlayerCharacter.Boundary,
		"RELATIONSHIP: world: " + bible.PlayerWorldConnection,
	}
	player = append(player, laws...)
	return []storyPeerCardDoc{
		{peerID: "narrator", text: strings.Join(narrator, "\n")},
		{peerID: "creative-writer", text: strings.Join(writer, "\n")},
		{peerID: bible.PlayerCharacter.PeerID, text: strings.Join(player, "\n")},
	}
}

func (w Workspace) WriteIndex(docs []IndexDocument, collection string) error {
	if err := os.MkdirAll(w.IndexDir(), 0o755); err != nil {
		return err
	}
	docs = enrichSourceHashes(w.Dir, docs)
	docPath := filepath.Join(w.IndexDir(), "documents.jsonl")
	tmp := docPath + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	enc := json.NewEncoder(writer)
	enc.SetEscapeHTML(false)
	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, docPath); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(w.IndexDir(), "manifest.json"), BuildIndexManifest(docs, collection))
}

func enrichSourceHashes(root string, docs []IndexDocument) []IndexDocument {
	out := append([]IndexDocument(nil), docs...)
	for i := range out {
		path := filepath.Join(root, filepath.FromSlash(out[i].SourcePath))
		if sum, err := hashFile(path); err == nil {
			out[i].SourceSHA256 = sum
		}
	}
	return out
}

func (w Workspace) ReadIndexDocuments() ([]IndexDocument, error) {
	path := filepath.Join(w.IndexDir(), "documents.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var docs []IndexDocument
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var doc IndexDocument
		if err := json.Unmarshal(scanner.Bytes(), &doc); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, scanner.Err()
}

func BuildIndexManifest(docs []IndexDocument, collection string) IndexManifest {
	counts := map[string]int{}
	sourceHashes := map[string]string{}
	manifestDocs := make([]IndexManifestItem, 0, len(docs))
	for _, doc := range docs {
		counts[doc.Kind]++
		sourceHashes[doc.SourcePath] = doc.SourceSHA256
		manifestDocs = append(manifestDocs, IndexManifestItem{
			DocID:      doc.DocID,
			Kind:       doc.Kind,
			Path:       doc.SourcePath,
			TextSHA256: doc.TextSHA256,
		})
	}
	var sources []IndexManifestItem
	for path, sha := range sourceHashes {
		sources = append(sources, IndexManifestItem{Path: path, SHA256: sha})
	}
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	return IndexManifest{
		GeneratedAt:    NowString(),
		Collection:     collection,
		EmbeddingModel: "voyage-4",
		DocumentCount:  len(docs),
		CountsByKind:   counts,
		Sources:        sources,
		Documents:      manifestDocs,
	}
}

func newIndexDocument(docID, kind, sourcePath, text, storyID, packetID, nodeID string, turn int, heading string, payload map[string]any) IndexDocument {
	text = normalizeText(text)
	return IndexDocument{
		DocID:        docID,
		PointID:      pointIDForDocID(docID),
		Kind:         kind,
		SourcePath:   sourcePath,
		SourceSHA256: hashText(sourcePath),
		TextSHA256:   hashText(text),
		Text:         text,
		StoryID:      storyID,
		PacketID:     packetID,
		NodeID:       nodeID,
		Turn:         turn,
		Heading:      heading,
		Payload:      payload,
	}
}

func storyBibleIndexText(bible StoryBible) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Story ID: %s\nTitle: %s\nPremise: %s\n", bible.StoryID, bible.Title, bible.Premise)
	fmt.Fprintf(&b, "Player: %s (%s)\n", bible.PlayerCharacter.Name, bible.PlayerCharacter.PeerID)
	fmt.Fprintf(&b, "Player description: %s\nDesire: %s\nWound: %s\nBoundary: %s\n", bible.PlayerCharacter.Description, bible.PlayerCharacter.Desire, bible.PlayerCharacter.Wound, bible.PlayerCharacter.Boundary)
	fmt.Fprintf(&b, "Player-world connection: %s\n", bible.PlayerWorldConnection)
	writeIndexList(&b, "World basics", bible.WorldBasics)
	writeIndexList(&b, "Core relationships", bible.CoreRelationships)
	writeIndexList(&b, "Tone doctrine", bible.ToneDoctrine)
	writeIndexList(&b, "First-turn exposition", bible.FirstTurnExposition)
	fmt.Fprintf(&b, "Resume summary: %s\n", bible.ResumeSummary)
	return b.String()
}

func lorePrincipleIndexText(bible StoryBible, law OriginalLaw) string {
	return strings.Join([]string{
		"Story lore principle: " + law.Name,
		"Story: " + bible.Title,
		"Description: " + law.Description,
		"Enforcement: " + law.Enforcement,
		"Why profound: " + law.WhyProfound,
		"Not merely cruel: " + law.NotMerelyCruel,
		"Source pattern: " + law.SourcePattern,
	}, "\n")
}

func packetIndexText(packet scene.StoryPacket) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Packet: %s\nEntry node: %s\nLocation: %s\nTime: %s\n", packet.ID, packet.EntryNodeID, packet.ContextSnapshot.Location, packet.ContextSnapshot.Time)
	for _, node := range packet.Nodes {
		fmt.Fprintf(&b, "\nNode: %s\nTitle: %s\n", node.ID, node.Title)
		for _, beat := range node.Beats {
			fmt.Fprintf(&b, "- %s\n", beat.Text)
		}
		for _, choice := range node.Choices {
			fmt.Fprintf(&b, "Choice %s: %s\n", choice.ID, choice.Label)
		}
	}
	return b.String()
}

func nodeIndexText(packet scene.StoryPacket, node scene.SceneNode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Packet: %s\nNode: %s\nTitle: %s\n", packet.ID, node.ID, node.Title)
	for _, beat := range node.Beats {
		fmt.Fprintf(&b, "Beat %s: %s\n", beat.ID, beat.Text)
	}
	for _, choice := range node.Choices {
		fmt.Fprintf(&b, "Choice %s -> %s: %s\n", choice.ID, choice.TargetNodeID, choice.Label)
	}
	for _, intent := range node.MemoryIntents {
		fmt.Fprintf(&b, "Memory intent %s: %s\n", intent.ID, intent.Summary)
	}
	return b.String()
}

func turnIndexText(turn TurnRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Turn: %d\nPacket: %s\nNode: %s\n", turn.Turn, turn.PacketID, turn.NodeID)
	if turn.Decision.ChoiceID != "" {
		fmt.Fprintf(&b, "Player choice: %s\n", turn.Decision.ChoiceID)
	}
	if turn.Decision.FreeText != "" {
		fmt.Fprintf(&b, "Player free text: %s\n", turn.Decision.FreeText)
	}
	fmt.Fprintf(&b, "Narrator prose:\n%s\n", turn.Prose)
	for _, event := range turn.Commit.Events {
		fmt.Fprintf(&b, "Canon event for %s: %s\n", event.PeerID, event.Content)
	}
	return b.String()
}

func writeIndexList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(b, "- %s\n", strings.TrimSpace(value))
		}
	}
}

func compactDocs(docs []IndexDocument) []IndexDocument {
	out := docs[:0]
	seen := map[string]struct{}{}
	for _, doc := range docs {
		if strings.TrimSpace(doc.Text) == "" {
			continue
		}
		if _, ok := seen[doc.DocID]; ok {
			continue
		}
		seen[doc.DocID] = struct{}{}
		out = append(out, doc)
	}
	return out
}

func normalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func pointIDForDocID(docID string) string {
	sum := sha256.Sum256([]byte("sans-nom:story-index:" + docID))
	raw := hex.EncodeToString(sum[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[:8], raw[8:12], raw[12:16], raw[16:20], raw[20:32])
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashText(string(data)), nil
}
