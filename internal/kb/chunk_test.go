package kb_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"sharedmemory/internal/kb"
)

func TestChunkText_Empty(t *testing.T) {
	if got := kb.ChunkText("", 1200); got != nil {
		t.Fatalf("empty → nil, got %#v", got)
	}
}

func TestChunkText_Short(t *testing.T) {
	chunks := kb.ChunkText("hello world", 1200)
	if len(chunks) != 1 {
		t.Fatalf("len = %d, want 1", len(chunks))
	}
	if chunks[0].Body != "hello world" {
		t.Fatalf("body = %q", chunks[0].Body)
	}
	if chunks[0].Body == "" {
		t.Fatal("body must be non-empty")
	}
}

func TestChunkText_LongMultiple(t *testing.T) {
	para := strings.Repeat("字", 500)
	text := para + "\n\n" + para + "\n\n" + para
	chunks := kb.ChunkText(text, 800)
	if len(chunks) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(chunks))
	}
	totalRunes := 0
	for i, c := range chunks {
		if c.Body == "" {
			t.Fatalf("chunk %d empty", i)
		}
		n := utf8.RuneCountInString(c.Body)
		if n > 800 {
			t.Fatalf("chunk %d has %d runes > max", i, n)
		}
		totalRunes += n
	}
	if totalRunes < utf8.RuneCountInString(text)/2 {
		t.Fatalf("chunks too small: covered %d runes of %d", totalRunes, utf8.RuneCountInString(text))
	}
}

func TestChunkText_HardSplitLongParagraph(t *testing.T) {
	text := strings.Repeat("a", 2500)
	chunks := kb.ChunkText(text, 1000)
	if len(chunks) < 3 {
		t.Fatalf("want >=3 hard-split chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Body == "" {
			t.Fatalf("chunk %d empty", i)
		}
		if utf8.RuneCountInString(c.Body) > 1000 {
			t.Fatalf("chunk %d exceeds maxRunes", i)
		}
	}
}
