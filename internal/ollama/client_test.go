package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("path = %q, want /api/embed", r.URL.Path)
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q, want test-model", req.Model)
		}
		if len(req.Input) != 2 {
			t.Errorf("input len = %d, want 2", len(req.Input))
		}
		resp := map[string]interface{}{
			"embeddings": [][]float32{
				{0.1, 0.2, 0.3},
				{0.4, 0.5, 0.6},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embeddings, err := Embed(context.Background(), server.URL, "test-model", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("got %d embeddings, want 2", len(embeddings))
	}
	if len(embeddings[0]) != 3 || embeddings[0][0] != 0.1 {
		t.Errorf("embeddings[0] = %v", embeddings[0])
	}
	if len(embeddings[1]) != 3 || embeddings[1][2] != 0.6 {
		t.Errorf("embeddings[1] = %v", embeddings[1])
	}
}

func TestEmbedEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call server for empty input")
	}))
	defer server.Close()

	embeddings, err := Embed(context.Background(), server.URL, "model", nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if embeddings != nil {
		t.Errorf("got %v, want nil", embeddings)
	}
}

func TestGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Stream {
			t.Error("stream should be false")
		}
		json.NewEncoder(w).Encode(map[string]string{"response": "Hello world"})
	}))
	defer server.Close()

	out, err := Generate(context.Background(), server.URL, "llama", "Say hello")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "Hello world" {
		t.Errorf("response = %q, want Hello world", out)
	}
}

func TestAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if !Available(context.Background(), server.URL) {
		t.Error("Available should be true for 200 response")
	}
}

func TestAvailableFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if Available(context.Background(), server.URL) {
		t.Error("Available should be false for 500 response")
	}
}
