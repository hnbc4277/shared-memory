package semantic_test

import (
	"testing"

	"sharedmemory/internal/semantic"
)

func TestFingerprint_Stable(t *testing.T) {
	a := semantic.Fingerprint("openai", "m", 2048, "http://x:1")
	b := semantic.Fingerprint("openai", "m", 2048, "http://x:1/")
	if a != b {
		t.Fatal("url normalize")
	}
}
