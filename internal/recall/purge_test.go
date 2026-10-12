package recall

import "testing"

func TestNormalizeRetentionDays(t *testing.T) {
	if normalizeRetentionDays(0) != 30 {
		t.Fatalf("0 -> %d", normalizeRetentionDays(0))
	}
	if normalizeRetentionDays(-5) != 30 {
		t.Fatalf("neg -> %d", normalizeRetentionDays(-5))
	}
	if normalizeRetentionDays(7) != 7 {
		t.Fatalf("7 -> %d", normalizeRetentionDays(7))
	}
	if normalizeRetentionDays(4000) != 3650 {
		t.Fatalf("cap -> %d", normalizeRetentionDays(4000))
	}
}
