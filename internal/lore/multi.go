package lore

import (
	"context"
	"sort"
)

type MultiRetriever struct {
	Retrievers []Retriever
}

func (r MultiRetriever) Search(ctx context.Context, query string, k int) ([]Snippet, error) {
	if k <= 0 {
		k = 5
	}
	seen := map[string]Snippet{}
	for _, retriever := range r.Retrievers {
		if retriever == nil {
			continue
		}
		hits, err := retriever.Search(ctx, query, k)
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			key := hit.ID
			if hit.Source != "" {
				key += "|" + hit.Source
			}
			if existing, ok := seen[key]; !ok || hit.Score > existing.Score {
				seen[key] = hit
			}
		}
	}
	out := make([]Snippet, 0, len(seen))
	for _, hit := range seen {
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}
