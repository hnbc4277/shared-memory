package hook

import "testing"

func TestExtractProvenance_CursorLike(t *testing.T) {
	ev := map[string]any{
		"model":     "gpt-4.1",
		"skills":    []any{"brainstorming"},
		"tool_name": "Shell",
	}
	p := extractProvenance(ev)
	if p.Model != "gpt-4.1" {
		t.Fatalf("model: %q", p.Model)
	}
	if len(p.Skills) != 1 || p.Skills[0] != "brainstorming" {
		t.Fatalf("skills: %#v", p.Skills)
	}
	if len(p.Tools) == 0 || p.Tools[0] != "Shell" {
		t.Fatalf("tools: %#v", p.Tools)
	}
	if len(p.Calls) == 0 || p.Calls[0]["name"] != "Shell" {
		t.Fatalf("calls: %#v", p.Calls)
	}
}

func TestExtractProvenance_Empty(t *testing.T) {
	p := extractProvenance(map[string]any{})
	if p.Model != "unknown" || p.Skills == nil || p.Plugins == nil || p.Tools == nil {
		t.Fatalf("%+v", p)
	}
}
