package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// LLMDeriver calls an OpenAI-compatible chat completions API; on any failure it
// falls back to Heuristic so worker jobs never fail solely due to LLM outages.
type LLMDeriver struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	Fallback   Deriver
}

func NewLLMDeriver(baseURL, apiKey, model string) *LLMDeriver {
	fb := Deriver(Heuristic{})
	return &LLMDeriver{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 45 * time.Second},
		Fallback:   fb,
	}
}

func (d *LLMDeriver) fallback() Deriver {
	if d.Fallback != nil {
		return d.Fallback
	}
	return Heuristic{}
}

func (d *LLMDeriver) Summarize(ctx context.Context, content string) (string, error) {
	prompt := "用一两句中文概括下面这段对话内容，不要编造，不要超过 280 字：\n\n" + Truncate(content, 6000)
	out, err := d.chat(ctx, prompt)
	if err != nil || strings.TrimSpace(out) == "" {
		if err != nil {
			log.Printf("llm deriver summarize: %v", err)
		}
		return d.fallback().Summarize(ctx, content)
	}
	return Truncate(Collapse(out), 280), nil
}

func (d *LLMDeriver) Extract(ctx context.Context, role, content string, hasProject bool) ([]Extracted, error) {
	scopeHint := "global"
	if hasProject {
		scopeHint = "project or global"
	}
	prompt := fmt.Sprintf(`从以下 %s 消息中抽取记忆条目。只输出 JSON 数组，每项形如 {"kind":"fact|decision|preference|entity","scope":"global|project","body":"..."}。
scope 可选：%s。没有可抽时输出 []。不要markdown。

消息：
%s`, role, scopeHint, Truncate(content, 6000))
	raw, err := d.chat(ctx, prompt)
	if err != nil {
		log.Printf("llm deriver extract: %v", err)
		return d.fallback().Extract(ctx, role, content, hasProject)
	}
	items, err := parseExtractedJSON(raw, hasProject)
	if err != nil || len(items) == 0 {
		if err != nil {
			log.Printf("llm deriver extract parse: %v", err)
		}
		return d.fallback().Extract(ctx, role, content, hasProject)
	}
	return items, nil
}

func parseExtractedJSON(raw string, hasProject bool) ([]Extracted, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no json array")
	}
	var rows []struct {
		Kind  string `json:"kind"`
		Scope string `json:"scope"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &rows); err != nil {
		return nil, err
	}
	out := []Extracted{}
	seen := map[string]bool{}
	for _, r := range rows {
		kind := strings.TrimSpace(r.Kind)
		scope := strings.TrimSpace(r.Scope)
		body := Truncate(Collapse(r.Body), 300)
		switch kind {
		case KindFact, KindDecision, KindPreference, KindEntity:
		default:
			continue
		}
		if scope != ScopeGlobal && scope != ScopeProject {
			if hasProject {
				scope = ScopeProject
			} else {
				scope = ScopeGlobal
			}
		}
		if !hasProject {
			scope = ScopeGlobal
		}
		key := kind + "|" + body
		if body == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Extracted{Kind: kind, Scope: scope, Body: body})
	}
	return out, nil
}

func (d *LLMDeriver) chat(ctx context.Context, userPrompt string) (string, error) {
	if d.BaseURL == "" {
		return "", fmt.Errorf("llm deriver url empty")
	}
	endpoint := d.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint = strings.TrimRight(endpoint, "/") + "/chat/completions"
	}
	body := map[string]any{
		"model": d.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You extract concise structured memory for a shared-memory system. Reply with plain text or JSON only."},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
	}
	client := d.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("llm http %s: %s", res.Status, Truncate(string(respBody), 200))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
