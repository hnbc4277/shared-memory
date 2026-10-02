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

const DefaultTextIndex = "sm_memories"

type OpenSearch struct {
	baseURL string
	index   string
	http    *http.Client
}

func NewOpenSearch(baseURL, index string) *OpenSearch {
	if index == "" {
		index = DefaultTextIndex
	}
	return &OpenSearch{baseURL: strings.TrimRight(baseURL, "/"), index: index, http: &http.Client{Timeout: 10 * time.Second}}
}

func (o *OpenSearch) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.baseURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("opensearch %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return out, resp.StatusCode, err
}

func (o *OpenSearch) Ensure(ctx context.Context) error {
	_, status, err := o.do(ctx, http.MethodHead, "/"+o.index, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	keyword := map[string]any{"type": "keyword"}
	mapping := map[string]any{
		"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0},
		"mappings": map[string]any{"properties": map[string]any{
			"doc_type":          keyword,
			"account_id":        keyword,
			"project_id":        keyword,
			"scope":             keyword,
			"kind":              keyword,
			"source_session_id": keyword,
			"source_agent_path": keyword,
			"body":              map[string]any{"type": "text", "analyzer": "cjk"},
			"created_at":        map[string]any{"type": "date"},
		}},
	}
	b, status, err := o.do(ctx, http.MethodPut, "/"+o.index, mapping)
	if err != nil {
		return err
	}
	if status >= 300 && !strings.Contains(string(b), "resource_already_exists") {
		return fmt.Errorf("create index: %d %s", status, b)
	}
	return nil
}

func (o *OpenSearch) Index(ctx context.Context, d Doc) error {
	b, status, err := o.do(ctx, http.MethodPut, fmt.Sprintf("/%s/_doc/%s?refresh=true", o.index, d.ID), d)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("index doc: %d %s", status, b)
	}
	return nil
}

func scopeFilter(q Query) []any {
	filters := []any{map[string]any{"term": map[string]any{"account_id": q.AccountID.String()}}}
	if q.ProjectID != nil {
		filters = append(filters, map[string]any{"bool": map[string]any{
			"should": []any{
				map[string]any{"term": map[string]any{"project_id": q.ProjectID.String()}},
				map[string]any{"bool": map[string]any{"must_not": map[string]any{"exists": map[string]any{"field": "project_id"}}}},
			},
			"minimum_should_match": 1,
		}})
	}
	return filters
}

func (o *OpenSearch) Search(ctx context.Context, q Query) ([]Hit, error) {
	if q.Limit <= 0 {
		q.Limit = 10
	}
	body := map[string]any{
		"size":    q.Limit,
		"_source": []string{"doc_type"},
		"query": map[string]any{"bool": map[string]any{
			"must":   []any{map[string]any{"match": map[string]any{"body": q.Text}}},
			"filter": scopeFilter(q),
		}},
	}
	b, status, err := o.do(ctx, http.MethodPost, "/"+o.index+"/_search", body)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("search: %d %s", status, b)
	}
	var res struct {
		Hits struct {
			Hits []struct {
				ID     string  `json:"_id"`
				Score  float64 `json:"_score"`
				Source struct {
					DocType string `json:"doc_type"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(res.Hits.Hits))
	for _, h := range res.Hits.Hits {
		id, err := uuid.Parse(h.ID)
		if err != nil {
			continue
		}
		out = append(out, Hit{ID: id, DocType: h.Source.DocType, Score: h.Score})
	}
	return out, nil
}

func (o *OpenSearch) deleteByQuery(ctx context.Context, filters []any) error {
	body := map[string]any{"query": map[string]any{"bool": map[string]any{"filter": filters}}}
	b, status, err := o.do(ctx, http.MethodPost, "/"+o.index+"/_delete_by_query?refresh=true&conflicts=proceed", body)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status >= 300 {
		return fmt.Errorf("delete_by_query: %d %s", status, b)
	}
	return nil
}

func (o *OpenSearch) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	return o.deleteByQuery(ctx, []any{
		map[string]any{"term": map[string]any{"account_id": accountID.String()}},
		map[string]any{"ids": map[string]any{"values": []string{id.String()}}},
	})
}

func (o *OpenSearch) DeleteProject(ctx context.Context, accountID, projectID uuid.UUID) error {
	return o.deleteByQuery(ctx, []any{
		map[string]any{"term": map[string]any{"account_id": accountID.String()}},
		map[string]any{"term": map[string]any{"project_id": projectID.String()}},
	})
}

func (o *OpenSearch) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	return o.deleteByQuery(ctx, []any{map[string]any{"term": map[string]any{"account_id": accountID.String()}}})
}
