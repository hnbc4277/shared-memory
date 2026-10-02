package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const DefaultCollection = "sm_memories"

// DefaultMinScore drops vector hits below this cosine similarity. With the
// 64-dim hash embedder, hash collisions give texts with no shared tokens
// scores up to ~0.15, while genuinely related texts score ~0.3+.
const DefaultMinScore = 0.25

type Qdrant struct {
	baseURL    string
	collection string
	dim        int
	MinScore   float64
	http       *http.Client
}

func NewQdrant(baseURL, collection string, dim int) *Qdrant {
	if collection == "" {
		collection = DefaultCollection
	}
	return &Qdrant{baseURL: strings.TrimRight(baseURL, "/"), collection: collection, dim: dim,
		MinScore: DefaultMinScore, http: &http.Client{Timeout: 10 * time.Second}}
}

func (q *Qdrant) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("qdrant %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return out, resp.StatusCode, err
}

func (q *Qdrant) Ensure(ctx context.Context) error {
	_, status, err := q.do(ctx, http.MethodGet, "/collections/"+q.collection, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	b, status, err := q.do(ctx, http.MethodPut, "/collections/"+q.collection, map[string]any{
		"vectors": map[string]any{"size": q.dim, "distance": "Cosine"},
	})
	if err != nil {
		return err
	}
	if status >= 300 && !strings.Contains(string(b), "already exists") {
		return fmt.Errorf("create collection: %d %s", status, b)
	}
	for _, field := range []string{"account_id", "project_id", "doc_type"} {
		_, _, _ = q.do(ctx, http.MethodPut, "/collections/"+q.collection+"/index?wait=true",
			map[string]any{"field_name": field, "field_schema": "keyword"})
	}
	return nil
}

func (q *Qdrant) Upsert(ctx context.Context, d Doc, vec []float32) error {
	payload := map[string]any{
		"doc_type":          d.DocType,
		"account_id":        d.AccountID.String(),
		"scope":             d.Scope,
		"kind":              d.Kind,
		"source_agent_path": d.SourceAgentPath,
	}
	if d.ProjectID != nil {
		payload["project_id"] = d.ProjectID.String()
	}
	b, status, err := q.do(ctx, http.MethodPut, "/collections/"+q.collection+"/points?wait=true", map[string]any{
		"points": []any{map[string]any{"id": d.ID.String(), "vector": vec, "payload": payload}},
	})
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("upsert: %d %s", status, b)
	}
	return nil
}

func match(key, value string) map[string]any {
	return map[string]any{"key": key, "match": map[string]any{"value": value}}
}

func (q *Qdrant) filter(query Query) map[string]any {
	f := map[string]any{"must": []any{match("account_id", query.AccountID.String())}}
	if query.ProjectID != nil {
		f["should"] = []any{
			match("project_id", query.ProjectID.String()),
			map[string]any{"is_empty": map[string]any{"key": "project_id"}},
		}
	}
	return f
}

func (q *Qdrant) Search(ctx context.Context, query Query) ([]Hit, error) {
	if query.Limit <= 0 {
		query.Limit = 10
	}
	body := map[string]any{
		"vector":       query.Vector,
		"limit":        query.Limit,
		"filter":       q.filter(query),
		"with_payload": []string{"doc_type"},
	}
	if q.MinScore > 0 {
		body["score_threshold"] = q.MinScore
	}
	b, status, err := q.do(ctx, http.MethodPost, "/collections/"+q.collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("search: %d %s", status, b)
	}
	var res struct {
		Result []struct {
			ID      string  `json:"id"`
			Score   float64 `json:"score"`
			Payload struct {
				DocType string `json:"doc_type"`
			} `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(res.Result))
	for _, r := range res.Result {
		id, err := uuid.Parse(r.ID)
		if err != nil {
			continue
		}
		out = append(out, Hit{ID: id, DocType: r.Payload.DocType, Score: r.Score})
	}
	return out, nil
}

func (q *Qdrant) deleteByFilter(ctx context.Context, must []any) error {
	b, status, err := q.do(ctx, http.MethodPost, "/collections/"+q.collection+"/points/delete?wait=true",
		map[string]any{"filter": map[string]any{"must": must}})
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 300 {
		return fmt.Errorf("delete: %d %s", status, b)
	}
	return nil
}

func (q *Qdrant) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	return q.deleteByFilter(ctx, []any{
		match("account_id", accountID.String()),
		map[string]any{"has_id": []string{id.String()}},
	})
}

func (q *Qdrant) DeleteProject(ctx context.Context, accountID, projectID uuid.UUID) error {
	return q.deleteByFilter(ctx, []any{match("account_id", accountID.String()), match("project_id", projectID.String())})
}

func (q *Qdrant) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	return q.deleteByFilter(ctx, []any{match("account_id", accountID.String())})
}
