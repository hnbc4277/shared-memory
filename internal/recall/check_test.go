package recall

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEvaluateReceiptCheck_NeedRecallWhenEmpty(t *testing.T) {
	got := evaluateReceiptCheck(nil, 30, time.Now())
	if got.OK || !got.NeedRecall {
		t.Fatalf("got %+v", got)
	}
	if got.WithinMinutes != 30 {
		t.Fatalf("within = %d", got.WithinMinutes)
	}
	if got.Latest != nil {
		t.Fatal("latest should be nil")
	}
	if got.Message == "" {
		t.Fatal("expected message")
	}
}

func TestEvaluateReceiptCheck_OKWhenRecent(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	r := Receipt{ID: id, QueriedAt: now.Add(-5 * time.Minute), HitCount: 2, Source: "mcp"}
	got := evaluateReceiptCheck([]Receipt{r}, 30, now)
	if !got.OK || got.NeedRecall {
		t.Fatalf("got %+v", got)
	}
	if got.Latest == nil || got.Latest.ID != id {
		t.Fatalf("latest = %+v", got.Latest)
	}
}

func TestEvaluateReceiptCheck_NeedRecallWhenStale(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	r := Receipt{ID: uuid.New(), QueriedAt: now.Add(-45 * time.Minute)}
	got := evaluateReceiptCheck([]Receipt{r}, 30, now)
	if got.OK || !got.NeedRecall {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeWithinMinutes(t *testing.T) {
	if normalizeWithinMinutes(0) != 30 {
		t.Fatal("default")
	}
	if normalizeWithinMinutes(-1) != 30 {
		t.Fatal("negative")
	}
	if normalizeWithinMinutes(15) != 15 {
		t.Fatal("custom")
	}
	if normalizeWithinMinutes(24*60+1) != 24*60 {
		t.Fatal("cap")
	}
}
