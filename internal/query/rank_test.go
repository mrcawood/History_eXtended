package query

import (
	"context"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	// Unit vectors: same direction
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if got := CosineSimilarity(a, b); got != 1 {
		t.Errorf("identical vectors: got %v, want 1", got)
	}
	// Orthogonal
	a = []float32{1, 0, 0}
	b = []float32{0, 1, 0}
	if got := CosineSimilarity(a, b); got != 0 {
		t.Errorf("orthogonal: got %v, want 0", got)
	}
	// Different lengths
	if got := CosineSimilarity([]float32{1}, []float32{1, 0}); got != 0 {
		t.Errorf("mismatched len: got %v, want 0", got)
	}
}

func TestRerankBySemantic(t *testing.T) {
	candidates := []Candidate{
		{EventID: 1, Cmd: "make test"},
		{EventID: 2, Cmd: "ls -la"},
		{EventID: 3, Cmd: "make clean"},
	}
	embedFn := func(ctx context.Context, texts []string) ([][]float32, error) {
		// Mock: L2-normalized vectors. question [1,0,0]; make test [1,0,0] sim=1; ls [0,1,0] sim=0; make clean [0.99,0.14,0] sim~0.99
		out := make([][]float32, len(texts))
		out[0] = []float32{1, 0, 0}
		out[1] = []float32{1, 0, 0}       // make test - highest sim
		out[2] = []float32{0, 1, 0}       // ls - lowest
		out[3] = []float32{0.99, 0.14, 0} // make clean - medium
		return out, nil
	}
	result, err := RerankBySemantic(context.Background(), "how to build", candidates, embedFn)
	if err != nil {
		t.Fatalf("RerankBySemantic: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3", len(result))
	}
	// First should be "make test" (highest sim), then "make clean", then "ls -la"
	if result[0].Cmd != "make test" {
		t.Errorf("first = %q, want make test", result[0].Cmd)
	}
}

func TestRerankBySemanticEmpty(t *testing.T) {
	result, err := RerankBySemantic(context.Background(), "q", nil, nil)
	if err != nil {
		t.Fatalf("RerankBySemantic: %v", err)
	}
	if result != nil {
		t.Errorf("got %v, want nil", result)
	}
}
