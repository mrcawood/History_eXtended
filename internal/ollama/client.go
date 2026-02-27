package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	embedTimeout  = 30 * time.Second
	generateTimeout = 60 * time.Second
	availableTimeout = 5 * time.Second
)

// Embed returns embeddings for the given texts. The first embedding corresponds to texts[0], etc.
// Uses Ollama POST /api/embed with input as array of strings.
func Embed(ctx context.Context, baseURL, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	reqBody := map[string]interface{}{
		"model": model,
		"input": texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	u, err := url.JoinPath(baseURL, "api/embed")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: embedTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed: %s: %s", resp.Status, string(b))
	}

	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d embeddings, expected %d", len(out.Embeddings), len(texts))
	}
	return out.Embeddings, nil
}

// Generate returns a single completion for the prompt. Uses stream: false.
func Generate(ctx context.Context, baseURL, model, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	u, err := url.JoinPath(baseURL, "api/generate")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: generateTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama generate: %s: %s", resp.Status, string(b))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Response, nil
}

// Available returns true if Ollama is reachable at baseURL. Uses GET /api/tags as a lightweight check.
func Available(ctx context.Context, baseURL string) bool {
	u, err := url.JoinPath(baseURL, "api/tags")
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: availableTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
