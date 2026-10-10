package recall

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFormat_IncludesKnowledgeBaseSection(t *testing.T) {
	res := Result{
		Profile: "likes go",
		Hits: []Item{{
			Kind: "fact", Scope: "global", Body: "mem",
			CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		}},
		KBHits: []KBHit{{ID: uuid.New(), Title: "Guide", Body: "chunk text"}},
	}
	ctx := format(res)
	if !strings.Contains(ctx, "## Knowledge Base") {
		t.Fatalf("missing KB heading: %q", ctx)
	}
	if !strings.Contains(ctx, "Guide") || !strings.Contains(ctx, "chunk text") {
		t.Fatalf("missing KB content: %q", ctx)
	}
}
