package lore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/telemetry"
)

type BridgeSearchClient interface {
	LoreSearch(ctx context.Context, req bridgeclient.LoreSearchRequest) (bridgeclient.LoreSearchResponse, error)
}

type BridgeRetriever struct {
	Client      BridgeSearchClient
	Telemetry   telemetry.Sink
	Kind        []string
	PeerID      []string
	CharacterID []string
	Volume      []int
}

func (r BridgeRetriever) Search(ctx context.Context, query string, k int) ([]Snippet, error) {
	if r.Client == nil {
		return nil, errors.New("lore bridge retriever requires a client")
	}
	start := time.Now()
	req := bridgeclient.LoreSearchRequest{
		Query:       query,
		K:           k,
		Kind:        r.Kind,
		PeerID:      r.PeerID,
		CharacterID: r.CharacterID,
		Volume:      r.Volume,
	}
	resp, err := r.Client.LoreSearch(ctx, req)
	snippets := make([]Snippet, 0, len(resp.Snippets))
	for _, hit := range resp.Snippets {
		snippets = append(snippets, Snippet{
			ID:          hit.DocID,
			Text:        hit.Text,
			Source:      hit.SourcePath,
			Score:       hit.Score,
			Kind:        hit.Kind,
			PageID:      hit.PageID,
			PeerID:      hit.PeerID,
			CharacterID: hit.CharacterID,
			Volume:      hit.Volume,
			Payload:     hit.Payload,
		})
	}
	emitLoreSearch(ctx, r.Telemetry, query, k, time.Since(start), snippets, err, r.filters())
	if err != nil {
		return nil, err
	}
	return snippets, nil
}

func emitLoreSearch(
	ctx context.Context,
	sink telemetry.Sink,
	query string,
	k int,
	latency time.Duration,
	snippets []Snippet,
	err error,
	filters map[string]any,
) {
	topScore := 0.0
	if len(snippets) > 0 {
		topScore = snippets[0].Score
	}
	metrics := telemetry.LoreSearchMetrics{
		QuerySHA256: queryHash(query),
		K:           k,
		Latency:     latency,
		HitCount:    len(snippets),
		TopScore:    topScore,
		Miss:        len(snippets) == 0,
		Filters:     filters,
	}
	if err != nil {
		metrics.Error = err.Error()
	}
	telemetry.EmitLoreSearch(ctx, sink, metrics)
}

func (r BridgeRetriever) filters() map[string]any {
	filters := map[string]any{}
	if len(r.Kind) > 0 {
		filters["kind"] = r.Kind
	}
	if len(r.PeerID) > 0 {
		filters["peer_id"] = r.PeerID
	}
	if len(r.CharacterID) > 0 {
		filters["character_id"] = r.CharacterID
	}
	if len(r.Volume) > 0 {
		filters["volume"] = r.Volume
	}
	return filters
}

func queryHash(query string) string {
	sum := sha256.Sum256([]byte(query))
	return hex.EncodeToString(sum[:])
}
