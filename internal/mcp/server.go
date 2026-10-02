// Package mcp implements a minimal Model Context Protocol server over stdio
// (newline-delimited JSON-RPC 2.0) that proxies tools to the REST API, so any
// MCP-capable agent can share memory without a dedicated adapter.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const ProtocolVersion = "2024-11-05"

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func (c *Client) call(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(out)))
	}
	if len(out) == 0 {
		out = []byte(`{"ok":true}`)
	}
	return out, nil
}

type Server struct {
	Client *Client
	// DefaultAgent fills `agent` for memory_ingest when the caller omits it.
	DefaultAgent string
	Version      string
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests until in is closed. Requests are handled sequentially,
// which keeps responses ordered and is plenty for one interactive agent.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var mu sync.Mutex
	enc := json.NewEncoder(out)
	write := func(r rpcResponse) {
		mu.Lock()
		defer mu.Unlock()
		_ = enc.Encode(r)
	}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			write(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // notification, e.g. notifications/initialized
		}
		result, rerr := s.dispatch(ctx, req)
		write(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr})
	}
	return sc.Err()
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "shared-memory", "version": s.Version},
			"instructions": "Shared memory across agents. Call memory_recall at the start of a task, and memory_ingest " +
				"for every user and assistant message with full provenance (agent, agent_path, skills, plugins, tools, model).",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefs}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		text, err := s.callTool(ctx, p.Name, p.Arguments)
		if err != nil {
			return map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *Server) callTool(ctx context.Context, name string, raw json.RawMessage) (string, error) {
	args := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("arguments must be an object")
		}
	}
	str := func(k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	var (
		out json.RawMessage
		err error
	)
	switch name {
	case "memory_ingest":
		if str("agent") == "" && s.DefaultAgent != "" {
			args["agent"] = s.DefaultAgent
		}
		out, err = s.Client.call(ctx, http.MethodPost, "/v1/ingest", args)
	case "memory_recall":
		out, err = s.Client.call(ctx, http.MethodPost, "/v1/recall", args)
	case "memory_list_sessions":
		q := url.Values{}
		for _, k := range []string{"project_id", "agent", "agent_path", "from", "to"} {
			if v := str(k); v != "" {
				q.Set(k, v)
			}
		}
		if v, ok := args["limit"].(float64); ok {
			q.Set("limit", fmt.Sprint(int(v)))
		}
		out, err = s.Client.call(ctx, http.MethodGet, "/v1/sessions?"+q.Encode(), nil)
	case "memory_get_session":
		id := str("session_id")
		if id == "" {
			return "", fmt.Errorf("session_id is required")
		}
		out, err = s.Client.call(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(id), nil)
	case "memory_list_candidates":
		out, err = s.Client.call(ctx, http.MethodGet, "/v1/candidates", nil)
	case "memory_resolve_candidate":
		kind, id, decision := str("kind"), str("candidate_id"), str("decision")
		if (kind != "vault" && kind != "artifact") || id == "" || (decision != "accept" && decision != "reject") {
			return "", fmt.Errorf("kind must be vault|artifact, decision accept|reject, candidate_id required")
		}
		prefix := "/v1/vault/candidates/"
		if kind == "artifact" {
			prefix = "/v1/artifacts/candidates/"
		}
		out, err = s.Client.call(ctx, http.MethodPost, prefix+url.PathEscape(id)+"/"+decision, map[string]any{})
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var stringList = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}

var toolDefs = []map[string]any{
	{
		"name": "memory_ingest",
		"description": "Record one chat message in shared memory with mandatory provenance. Send skills/plugins/tools as [] " +
			"when none were used. For sub-agents set agent_path like 'xiaolongxia/researcher'. Omit session_id for the " +
			"first message, then reuse the returned session_id. If the response contains prompts, ask the user.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"}, "project_id": map[string]any{"type": "string"},
				"title": map[string]any{"type": "string"}, "agent": map[string]any{"type": "string"},
				"agent_path": map[string]any{"type": "string"}, "skills": stringList, "plugins": stringList, "tools": stringList,
				"model":   map[string]any{"type": "string"},
				"role":    map[string]any{"type": "string", "enum": []string{"user", "assistant", "system", "tool"}},
				"content": map[string]any{"type": "string"}, "attachment_ids": stringList,
				"store_transcript_mark": map[string]any{"type": "boolean"}, "artifact_offer": map[string]any{"type": "boolean"},
			},
			"required": []string{"skills", "plugins", "tools", "role", "content"},
		},
	},
	{
		"name":        "memory_recall",
		"description": "Retrieve the user's shared memory (profile, project summary, relevant memories) for a query. Use the returned 'context' as background.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"}, "project_id": map[string]any{"type": "string"},
				"top_k": map[string]any{"type": "integer"},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "memory_list_sessions",
		"description": "List the user's chat sessions across all agents, newest first. Filter by agent or agent_path (includes sub-agents).",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string"}, "agent": map[string]any{"type": "string"},
				"agent_path": map[string]any{"type": "string"}, "from": map[string]any{"type": "string"},
				"to": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"},
			},
		},
	},
	{
		"name":        "memory_get_session",
		"description": "Get one session with every message and its provenance (agent_path, skills, plugins, tools, model).",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"session_id": map[string]any{"type": "string"}},
			"required":   []string{"session_id"},
		},
	},
	{
		"name":        "memory_list_candidates",
		"description": "List pending items awaiting the user's decision: detected credentials (vault) and generated documents (artifacts).",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name": "memory_resolve_candidate",
		"description": "Accept or reject a pending vault/artifact candidate. Only call this after the user explicitly " +
			"agreed in the conversation; never decide on the user's behalf.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":         map[string]any{"type": "string", "enum": []string{"vault", "artifact"}},
				"candidate_id": map[string]any{"type": "string"},
				"decision":     map[string]any{"type": "string", "enum": []string{"accept", "reject"}},
			},
			"required": []string{"kind", "candidate_id", "decision"},
		},
	},
}
