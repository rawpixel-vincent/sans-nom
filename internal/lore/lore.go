package lore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"strings"
)

type Snippet struct {
	ID     string  `json:"id"`
	Text   string  `json:"text"`
	Source string  `json:"source"`
	Score  float64 `json:"score"`
}

type Retriever interface {
	Search(ctx context.Context, query string, k int) ([]Snippet, error)
}

type Embedder interface {
	Embed(ctx context.Context, input string) ([]float64, error)
}

type MemoryIndex struct {
	embedder Embedder
	docs     []indexedDoc
}

type indexedDoc struct {
	snippet Snippet
	vector  []float64
}

func NewMemoryIndex(ctx context.Context, embedder Embedder, snippets []Snippet) (*MemoryIndex, error) {
	idx := &MemoryIndex{embedder: embedder}
	for _, snippet := range snippets {
		vector, err := embedder.Embed(ctx, snippet.Text)
		if err != nil {
			return nil, err
		}
		idx.docs = append(idx.docs, indexedDoc{snippet: snippet, vector: vector})
	}
	return idx, nil
}

func (i *MemoryIndex) Search(ctx context.Context, query string, k int) ([]Snippet, error) {
	if k <= 0 {
		k = 5
	}
	queryVec, err := i.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	results := make([]Snippet, 0, len(i.docs))
	for _, doc := range i.docs {
		snippet := doc.snippet
		snippet.Score = cosine(queryVec, doc.vector)
		results = append(results, snippet)
	}
	sort.Slice(results, func(a, b int) bool {
		return results[a].Score > results[b].Score
	})
	if len(results) > k {
		results = results[:k]
	}
	return results, nil
}

type FakeEmbedder struct {
	Dimensions int
}

func (e FakeEmbedder) Embed(_ context.Context, input string) ([]float64, error) {
	dims := e.Dimensions
	if dims == 0 {
		dims = 16
	}
	vector := make([]float64, dims)
	for _, token := range strings.Fields(strings.ToLower(input)) {
		sum := sha256.Sum256([]byte(token))
		idx := int(binary.BigEndian.Uint64(sum[:8]) % uint64(dims))
		vector[idx] += 1
	}
	return normalize(vector), nil
}

func cosine(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
	}
	return dot
}

func normalize(v []float64) []float64 {
	var mag float64
	for _, x := range v {
		mag += x * x
	}
	mag = math.Sqrt(mag)
	if mag == 0 {
		return v
	}
	for i := range v {
		v[i] /= mag
	}
	return v
}
