package recall

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPreviewQuery_TruncatesRunes(t *testing.T) {
	long := strings.Repeat("中", 250)
	got := previewQuery(long, 200)
	if utf8Count := len([]rune(got)); utf8Count != 200 {
		t.Fatalf("rune count = %d, want 200", utf8Count)
	}
	if got != string([]rune(long)[:200]) {
		t.Fatal("unexpected truncation")
	}
}

func TestBuildReceipt_DegradedAndDefaults(t *testing.T) {
	pid := uuid.New()
	req := Request{
		Query:     "  hello  ",
		ProjectID: &pid,
		Source:    "",
	}
	res := Result{
		Hits:     []Item{{ID: uuid.New()}, {ID: uuid.New()}},
		KBHits:   []KBHit{{ID: uuid.New()}, {ID: uuid.New()}, {ID: uuid.New()}},
		Warnings: []string{WarnPGFallback},
	}
	r := buildReceipt(req, res)
	if !r.Degraded {
		t.Fatal("expected degraded for pg_fallback")
	}
	if r.Source != "http" {
		t.Fatalf("source = %q", r.Source)
	}
	if r.HitCount != 2 {
		t.Fatalf("hit_count = %d", r.HitCount)
	}
	if r.KBHitCount != 3 {
		t.Fatalf("kb_hit_count = %d", r.KBHitCount)
	}
	if r.QueryPreview != "hello" {
		t.Fatalf("query_preview = %q", r.QueryPreview)
	}
	if r.Warnings == nil {
		t.Fatal("warnings must be non-nil")
	}
}

func TestIsDegraded_IncludesKBDegraded(t *testing.T) {
	if !isDegraded([]string{WarnKBDegraded}) {
		t.Fatal("WarnKBDegraded must count as degraded")
	}
	if isDegraded([]string{WarnInjectOff}) {
		t.Fatal("inject_disabled alone must not be degraded")
	}
}
