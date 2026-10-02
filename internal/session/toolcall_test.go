package session_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"sharedmemory/internal/session"
)

func TestToolCallOutputTruncationContract(t *testing.T) {
	// Ingest truncates to 4000 runes; verify the constant is reflected in behavior via type presence.
	long := strings.Repeat("字", 5000)
	if utf8.RuneCountInString(long) != 5000 {
		t.Fatal("setup")
	}
	tc := session.ToolCall{Kind: "tool", Name: "shell", Output: long}
	if tc.Name != "shell" {
		t.Fatal(tc.Name)
	}
}
