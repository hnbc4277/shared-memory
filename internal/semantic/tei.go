package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BGEQueryInstruction is the retrieval prefix recommended for bge-*-zh models;
// it is applied to queries only, never to stored passages.
const BGEQueryInstruction = "为这个句子生成表示以用于检索相关文章："

// QueryEmbedder is implemented by embedders that encode search queries
// differently from stored passages.
type QueryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// TEIEmbedder calls a HuggingFace text-embeddings-inference server.
type TEIEmbedder struct {
	baseURL     string
	dim         int
	QueryPrefix string
	http        *http.Client
}

func NewTEIEmbedder(baseURL string, dim int) *TEIEmbedder {
	return &TEIEmbedder{baseURL: strings.TrimRight(baseURL, "/"), dim: dim, QueryPrefix: BGEQueryInstruction,
		http: &http.Client{Timeout: 5 * time.Second}}
}

func (e *TEIEmbedder) Dim() int { return e.dim }

func (e *TEIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.embed(ctx, text)
}

func (e *TEIEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return e.embed(ctx, e.QueryPrefix+text)
}

func (e *TEIEmbedder) embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		text = " "
	}
	body, _ := json.Marshal(map[string]any{"inputs": text, "truncate": true, "normalize": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedder: %d %s", resp.StatusCode, b)
	}
	var out [][]float32
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("embedder response: %w", err)
	}
	if len(out) != 1 {
		return nil, fmt.Errorf("embedder: expected 1 vector, got %d", len(out))
	}
	got := len(out[0])
	if got == 0 {
		return nil, fmt.Errorf("embedder returned dim 0, want positive")
	}
	if e.dim > 0 && got != e.dim {
		return nil, fmt.Errorf("embedder returned dim %d, want %d", got, e.dim)
	}
	return out[0], nil
}
