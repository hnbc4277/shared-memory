package index_test

import (
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/index"
)

func TestFuse_RewardsDocsRankedByBothEngines(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	text := []index.Hit{{ID: a}, {ID: b}}
	vec := []index.Hit{{ID: b}, {ID: c}}
	got := index.Fuse(10, text, vec)
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].ID != b {
		t.Fatalf("doc in both lists must rank first, got %v", got[0].ID)
	}
	if got := index.Fuse(1, text, vec); len(got) != 1 {
		t.Fatalf("limit not applied: %d", len(got))
	}
	if got := index.Fuse(5); len(got) != 0 {
		t.Fatalf("empty input: %v", got)
	}
}
