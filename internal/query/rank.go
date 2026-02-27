package query

import (
	"context"
	"sort"
)

// Candidate is an event candidate for ranking.
type Candidate struct {
	EventID   int64
	SessionID string
	Seq       int
	Cmd       string
	Cwd       string
	ExitCode  int
}

// CosineSimilarity returns the cosine similarity between two L2-normalized vectors.
// Ollama embeddings are L2-normalized, so dot product equals cosine similarity.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i] * b[i])
	}
	return float32(dot)
}

// EmbedFn embeds texts and returns one vector per text, in order.
type EmbedFn func(ctx context.Context, texts []string) ([][]float32, error)

// RerankBySemantic embeds the question and candidate cmd_texts, computes cosine similarity,
// and returns candidates sorted by similarity descending.
func RerankBySemantic(ctx context.Context, question string, candidates []Candidate, embed EmbedFn) ([]Candidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	texts := make([]string, 0, len(candidates)+1)
	texts = append(texts, question)
	for _, c := range candidates {
		texts = append(texts, c.Cmd)
	}
	embeddings, err := embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(embeddings) != len(texts) {
		return candidates, nil
	}
	queryVec := embeddings[0]
	type scored struct {
		c     Candidate
		score float32
	}
	scoredList := make([]scored, len(candidates))
	for i, c := range candidates {
		score := float32(0)
		if i+1 < len(embeddings) {
			score = CosineSimilarity(queryVec, embeddings[i+1])
		}
		scoredList[i] = scored{c: c, score: score}
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})
	out := make([]Candidate, len(candidates))
	for i := range scoredList {
		out[i] = scoredList[i].c
	}
	return out, nil
}
