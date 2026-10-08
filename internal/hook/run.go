package hook

import (
	"context"
	"encoding/json"
	"io"
	"strings"
)

// Run dispatches stdin JSON for the given surface to stdout JSON.
// agentOverride (SHARED_MEMORY_AGENT) renames the agent field, e.g. deepseek-harness on the Codex surface.
func Run(ctx context.Context, surface string, api API, in io.Reader, out io.Writer) error {
	return RunWithAgent(ctx, surface, "", api, in, out)
}

func RunWithAgent(ctx context.Context, surface, agentOverride string, api API, in io.Reader, out io.Writer) error {
	surface = strings.ToLower(strings.TrimSpace(surface))
	raw, err := io.ReadAll(io.LimitReader(in, 8<<20))
	if err != nil {
		return err
	}
	raw = bytesTrim(raw)
	if len(raw) == 0 {
		return writeJSON(out, map[string]any{})
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return writeJSON(out, map[string]any{})
	}
	switch surface {
	case "codex":
		agent := AgentCodex
		if a := strings.TrimSpace(agentOverride); a != "" {
			agent = a
		}
		return handleCodex(ctx, api, agent, payload, out)
	default:
		agent := AgentCursor
		if a := strings.TrimSpace(agentOverride); a != "" {
			agent = a
		}
		return handleCursor(ctx, api, agent, payload, out)
	}
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func handleCursor(ctx context.Context, api API, agent string, m map[string]any, out io.Writer) error {
	ev := eventName(m)
	ext := externalSessionID(agent, m)
	switch ev {
	case "beforeSubmitPrompt":
		content, _ := stringField(m, "prompt")
		if content == "" {
			content, _ = stringField(m, "content")
		}
		_ = api.Ingest(ctx, ingestBody(agent, agent, "user", content, ext, m))
		return writeJSON(out, map[string]any{})
	case "afterAgentResponse":
		content, _ := stringField(m, "text")
		if content == "" {
			content, _ = stringField(m, "response")
		}
		if content == "" {
			content, _ = stringField(m, "content")
		}
		_ = api.Ingest(ctx, ingestBody(agent, agent, "assistant", content, ext, m))
		return writeJSON(out, map[string]any{})
	case "subagentStop":
		sub, _ := stringField(m, "subagent_type")
		if sub == "" {
			sub, _ = stringField(m, "subagentType")
		}
		path := agent
		if sub != "" {
			path = agent + "/" + sub
		}
		content, _ := stringField(m, "summary")
		if content == "" {
			content, _ = stringField(m, "result")
		}
		if content == "" {
			content, _ = stringField(m, "text")
		}
		_ = api.Ingest(ctx, ingestBody(agent, path, "assistant", content, ext, m))
		return writeJSON(out, map[string]any{})
	case "sessionStart":
		q := "session start"
		if t, ok := stringField(m, "composer_mode"); ok {
			q = t
		}
		ctxText, err := api.Recall(ctx, q)
		resp := map[string]any{}
		if err == nil && strings.TrimSpace(ctxText) != "" {
			resp["additional_context"] = TruncateContext(ctxText)
		}
		return writeJSON(out, resp)
	default:
		return writeJSON(out, map[string]any{})
	}
}

func handleCodex(ctx context.Context, api API, agent string, m map[string]any, out io.Writer) error {
	ev := eventName(m)
	ext := externalSessionID(agent, m)
	switch ev {
	case "SessionStart":
		q := "session"
		if p, ok := stringField(m, "prompt"); ok && p != "" {
			q = p
		}
		ctxText, _ := api.Recall(ctx, q)
		return writeJSON(out, map[string]any{
			"hookSpecificOutput": map[string]any{
				"additionalContext": TruncateContext(ctxText),
			},
		})
	case "UserPromptSubmit":
		prompt, _ := stringField(m, "prompt")
		if prompt == "" {
			prompt, _ = stringField(m, "content")
		}
		_ = api.Ingest(ctx, ingestBody(agent, agent, "user", prompt, ext, m))
		ctxText, _ := api.Recall(ctx, firstNonEmpty(prompt, "session"))
		return writeJSON(out, map[string]any{
			"hookSpecificOutput": map[string]any{
				"additionalContext": TruncateContext(ctxText),
			},
		})
	case "Stop":
		content, _ := stringField(m, "last_assistant_message")
		if content == "" {
			content, _ = stringField(m, "text")
		}
		_ = api.Ingest(ctx, ingestBody(agent, agent, "assistant", content, ext, m))
		return writeJSON(out, map[string]any{})
	case "SubagentStop":
		sub, _ := stringField(m, "agent_type")
		if sub == "" {
			sub, _ = stringField(m, "subagent_type")
		}
		path := agent
		if sub != "" {
			path = agent + "/" + sub
		}
		content, _ := stringField(m, "last_assistant_message")
		if content == "" {
			content, _ = stringField(m, "summary")
		}
		_ = api.Ingest(ctx, ingestBody(agent, path, "assistant", content, ext, m))
		return writeJSON(out, map[string]any{})
	default:
		return writeJSON(out, map[string]any{})
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func ingestBody(agent, agentPath, role, content, externalSessionID string, event map[string]any) map[string]any {
	prov := extractProvenance(event)
	body := map[string]any{
		"agent":      agent,
		"agent_path": agentPath,
		"role":       role,
		"content":    content,
		"skills":     prov.Skills,
		"plugins":    prov.Plugins,
		"tools":      prov.Tools,
		"model":      prov.Model,
	}
	if len(prov.Calls) > 0 {
		body["tool_calls"] = prov.Calls
	}
	if externalSessionID != "" {
		body["external_session_id"] = externalSessionID
	}
	return body
}
