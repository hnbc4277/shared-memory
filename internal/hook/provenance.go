package hook

import (
	"encoding/json"
	"strings"
)

type provenance struct {
	Model   string
	Skills  []string
	Plugins []string
	Tools   []string
	Calls   []map[string]any
}

func extractProvenance(event map[string]any) provenance {
	p := provenance{
		Model:   "unknown",
		Skills:  []string{},
		Plugins: []string{},
		Tools:   []string{},
		Calls:   []map[string]any{},
	}
	if event == nil {
		return p
	}
	if m := firstString(event, "model", "model_name", "modelName"); m != "" {
		p.Model = m
	}
	p.Skills = stringList(event, "skills", "skill")
	p.Plugins = stringList(event, "plugins", "plugin")
	p.Tools = stringList(event, "tools", "tool")
	if name := firstString(event, "tool_name", "toolName"); name != "" {
		p.Tools = appendUnique(p.Tools, name)
		p.Calls = append(p.Calls, map[string]any{"kind": "tool", "name": name})
	}
	if raw, ok := event["tool_calls"]; ok {
		for _, item := range asSlice(raw) {
			if m, ok := item.(map[string]any); ok {
				name := firstString(m, "name", "tool_name", "toolName")
				if name == "" {
					continue
				}
				kind := firstString(m, "kind")
				if kind == "" {
					kind = "tool"
				}
				p.Tools = appendUnique(p.Tools, name)
				call := map[string]any{"kind": kind, "name": name}
				if s := firstString(m, "started_at", "startedAt"); s != "" {
					call["started_at"] = s
				}
				if s := firstString(m, "ended_at", "endedAt"); s != "" {
					call["ended_at"] = s
				}
				if s := firstString(m, "output"); s != "" {
					call["output"] = s
				}
				p.Calls = append(p.Calls, call)
			}
		}
	}
	return p
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			case json.Number:
				return t.String()
			}
		}
	}
	return ""
}

func stringList(m map[string]any, keys ...string) []string {
	out := []string{}
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				out = appendUnique(out, s)
			}
		default:
			for _, item := range asSlice(t) {
				if s, ok := item.(string); ok {
					if s = strings.TrimSpace(s); s != "" {
						out = appendUnique(out, s)
					}
				}
			}
		}
	}
	return out
}

func asSlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	default:
		return nil
	}
}

func appendUnique(in []string, s string) []string {
	for _, x := range in {
		if x == s {
			return in
		}
	}
	return append(in, s)
}
