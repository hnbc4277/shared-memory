package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAIEmbedder calls an OpenAI-compatible embeddings API.
type OpenAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	dim     int
	http    *http.Client
}

func NewOpenAIEmbedder(baseURL, apiKey, model string, dim int) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		dim:     dim,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *OpenAIEmbedder) Dim() int { return e.dim }

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		text = " "
	}
	body, _ := json.Marshal(map[string]string{"model": e.model, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEmbeddingsURL(e.baseURL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai embedder: %d %s", resp.StatusCode, b)
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("openai embedder response: %w", err)
	}
	if len(out.Data) != 1 {
		return nil, fmt.Errorf("openai embedder: expected 1 embedding, got %d", len(out.Data))
	}
	raw := out.Data[0].Embedding
	got := len(raw)
	if got == 0 {
		return nil, fmt.Errorf("openai embedder returned empty vector")
	}
	if e.dim > 0 && got != e.dim {
		return nil, fmt.Errorf("openai embedder returned dim %d, want %d", got, e.dim)
	}
	vec := make([]float32, got)
	for i, v := range raw {
		vec[i] = float32(v)
	}
	return vec, nil
}

func openAIEmbeddingsURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	u, err := url.Parse(base)
	if err != nil {
		if strings.HasSuffix(base, "/v1") || strings.Contains(base, "/v1") {
			return base + "/embeddings"
		}
		return base + "/v1/embeddings"
	}
	path := u.Path
	if strings.HasSuffix(base, "/v1") || strings.Contains(path, "/v1") {
		return base + "/embeddings"
	}
	return base + "/v1/embeddings"
}
