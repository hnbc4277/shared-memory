// Package mcp implements a minimal Model Context Protocol server over stdio
// (newline-delimited JSON-RPC 2.0) that proxies tools to the REST API via the
// shared Go SDK, so any MCP-capable agent can share memory without a dedicated adapter.
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
	"log"
	"strings"
	"sync"
	"time"

	"sharedmemory/sdk/go/sm"
)

const ProtocolVersion = "2024-11-05"

// Client is retained as a thin alias so existing call sites keep compiling.
type Client = sm.Client

type Server struct {
	Client *sm.Client
	// DefaultAgent fills `agent` for memory_ingest when the caller omits it.
	DefaultAgent string
	// DefaultConnectionID is sent as connection_id on ingest/recall when omitted (env SHARED_MEMORY_CONNECTION_ID).
	DefaultConnectionID string
	// HeartbeatEvery controls connection presence POST interval; zero means 30s (tests may set ~20ms).
	HeartbeatEvery time.Duration
	Version        string
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
func (s *Server) heartbeatInterval() time.Duration {
	if s.HeartbeatEvery > 0 {
		return s.HeartbeatEvery
	}
	return 30 * time.Second
}

func (s *Server) startConnectionHeartbeat(ctx context.Context) {
	if s.Client == nil || s.HeartbeatEvery < 0 {
		return
	}
	every := s.heartbeatInterval()
	go func() {
		ping := func() {
			if err := PostConnectionHeartbeat(s.Client, ctx, s.DefaultConnectionID); err != nil {
				log.Printf("connection heartbeat: %v", err)
			}
		}
		ping()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				ping()
			}
		}
	}()
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.startConnectionHeartbeat(ctx)
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
			"instructions": "Shared memory across agents. Call memory_recall at the start of a task (returns memories and " +
				"knowledge base excerpts in context). Use kb_add to add text or promote artifacts/attachments into the " +
				"knowledge base. Call memory_ingest for every user and assistant message with full provenance.",
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

func (s *Server) applyDefaultConnectionID(args map[string]any) {
	if s.DefaultConnectionID == "" {
		return
	}
	if v, ok := args["connection_id"].(string); ok && strings.TrimSpace(v) != "" {
		return
	}
	args["connection_id"] = s.DefaultConnectionID
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
		s.applyDefaultConnectionID(args)
		out, err = s.Client.CallRaw(ctx, http.MethodPost, "/v1/ingest", args)
	case "memory_recall":
		if _, ok := args["source"]; !ok {
			args["source"] = "mcp"
		}
		s.applyDefaultConnectionID(args)
		out, err = s.Client.CallRaw(ctx, http.MethodPost, "/v1/recall", args)
	case "agents_list":
		out, err = s.Client.CallRaw(ctx, http.MethodGet, "/v1/agents", nil)
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
		path := "/v1/sessions"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		out, err = s.Client.CallRaw(ctx, http.MethodGet, path, nil)
	case "memory_get_session":
		id := str("session_id")
		if id == "" {
			return "", fmt.Errorf("session_id is required")
		}
		out, err = s.Client.CallRaw(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(id), nil)
	case "memory_recall_status":
		q := url.Values{}
		if v := str("external_session_id"); v != "" {
			q.Set("external_session_id", v)
		}
		if v := str("session_id"); v != "" {
			q.Set("session_id", v)
		}
		if v, ok := args["within_minutes"].(float64); ok && v > 0 {
			q.Set("within_minutes", fmt.Sprint(int(v)))
		}
		path := "/v1/recall/receipts/check"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		out, err = s.Client.CallRaw(ctx, http.MethodGet, path, nil)
	case "memory_list_candidates":
		out, err = s.Client.CallRaw(ctx, http.MethodGet, "/v1/candidates", nil)
	case "memory_resolve_candidate":
		kind, id, decision := str("kind"), str("candidate_id"), str("decision")
		if (kind != "vault" && kind != "artifact") || id == "" || (decision != "accept" && decision != "reject") {
			return "", fmt.Errorf("kind must be vault|artifact, decision accept|reject, candidate_id required")
		}
		prefix := "/v1/vault/candidates/"
		if kind == "artifact" {
			prefix = "/v1/artifacts/candidates/"
		}
		out, err = s.Client.CallRaw(ctx, http.MethodPost, prefix+url.PathEscape(id)+"/"+decision, map[string]any{})
	case "kb_add":
		payload := map[string]any{}
		if t := str("title"); t != "" {
			payload["title"] = t
		}
		if t := str("text"); t != "" {
			payload["text"] = t
			if m := str("mime"); m != "" {
				payload["mime"] = m
			}
		} else if id := str("artifact_id"); id != "" {
			payload["artifact_id"] = id
		} else if id := str("attachment_id"); id != "" {
			payload["attachment_id"] = id
		} else {
			return "", fmt.Errorf("kb_add requires text, artifact_id, or attachment_id")
		}
		out, err = s.Client.CallRaw(ctx, http.MethodPost, "/v1/kb/documents", payload)
	case "kb_list":
		q := url.Values{}
		if v, ok := args["limit"].(float64); ok && v > 0 {
			q.Set("limit", fmt.Sprint(int(v)))
		}
		if v, ok := args["offset"].(float64); ok && v >= 0 {
			q.Set("offset", fmt.Sprint(int(v)))
		}
		path := "/v1/kb/documents"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		out, err = s.Client.CallRaw(ctx, http.MethodGet, path, nil)
	case "kb_get":
		id := str("document_id")
		if id == "" {
			return "", fmt.Errorf("document_id is required")
		}
		out, err = s.Client.CallRaw(ctx, http.MethodGet, "/v1/kb/documents/"+url.PathEscape(id), nil)
	case "kb_delete":
		id := str("document_id")
		if id == "" {
			return "", fmt.Errorf("document_id is required")
		}
		out, err = s.Client.CallRaw(ctx, http.MethodDelete, "/v1/kb/documents/"+url.PathEscape(id), nil)
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
			"first message, then reuse the returned session_id. Prefer external_session_id for hook/daemon clients. " +
			"If the response contains prompts, ask the user.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"}, "external_session_id": map[string]any{"type": "string"},
				"project_id": map[string]any{"type": "string"},
				"title":      map[string]any{"type": "string"}, "agent": map[string]any{"type": "string"},
				"agent_path": map[string]any{"type": "string"}, "skills": stringList, "plugins": stringList, "tools": stringList,
				"model":   map[string]any{"type": "string"},
				"role":    map[string]any{"type": "string", "enum": []string{"user", "assistant", "system", "tool"}},
				"content": map[string]any{"type": "string"}, "attachment_ids": stringList,
				"store_transcript_mark": map[string]any{"type": "boolean"}, "artifact_offer": map[string]any{"type": "boolean"},
				"connection_id": map[string]any{"type": "string", "description": "Optional registered connection UUID from Web 智能体 tab"},
			},
			"required": []string{"skills", "plugins", "tools", "role", "content"},
		},
	},
	{
		"name": "memory_recall",
		"description": "Retrieve shared memory and knowledge base excerpts (profile, project summary, memories, kb_hits) for a query. " +
			"Use the returned 'context' as background. Call this at the start of every non-trivial task.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"}, "project_id": map[string]any{"type": "string"},
				"top_k": map[string]any{"type": "integer"},
				"external_session_id": map[string]any{"type": "string"},
				"connection_id":     map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "agents_list",
		"description": "List agent products and connection instances (online status, last activity). Same as Web 智能体 tab.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name": "memory_recall_status",
		"description": "Check whether this conversation already has a successful Recall Receipt in the last N minutes. " +
			"If need_recall is true, you MUST call memory_recall before continuing non-trivial work.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"external_session_id": map[string]any{"type": "string"},
				"session_id":          map[string]any{"type": "string"},
				"within_minutes":      map[string]any{"type": "integer"},
			},
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
	{
		"name": "kb_add",
		"description": "Add content to the account knowledge base: inline text (title optional), or promote an existing artifact_id or attachment_id. Large files should use the Web console upload.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":         map[string]any{"type": "string"},
				"text":          map[string]any{"type": "string"},
				"mime":          map[string]any{"type": "string"},
				"artifact_id":   map[string]any{"type": "string"},
				"attachment_id": map[string]any{"type": "string"},
			},
		},
	},
	{
		"name":        "kb_list",
		"description": "List knowledge base documents for the account (newest first).",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer"},
				"offset": map[string]any{"type": "integer"},
			},
		},
	},
	{
		"name":        "kb_get",
		"description": "Get one knowledge base document by document_id (status, errors, metadata).",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"document_id": map[string]any{"type": "string"}},
			"required":   []string{"document_id"},
		},
	},
	{
		"name":        "kb_delete",
		"description": "Delete a knowledge base document and its indexed chunks.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"document_id": map[string]any{"type": "string"}},
			"required":   []string{"document_id"},
		},
	},
}
