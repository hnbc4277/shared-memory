package kb

import (
	"strings"
	"unicode/utf8"
)

// ChunkText splits text into sequential chunks. Ordinal is left unset (0);
// callers assign document IDs and ordinals when persisting.
func ChunkText(text string, maxRunes int) []Chunk {
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = DefaultMaxRunes
	}
	paras := strings.Split(text, "\n\n")
	var out []Chunk
	var buf strings.Builder
	bufRunes := 0
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		out = append(out, Chunk{Body: buf.String()})
		buf.Reset()
		bufRunes = 0
	}
	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pr := utf8.RuneCountInString(p)
		if pr > maxRunes {
			flush()
			for _, part := range hardSplitRunes(p, maxRunes) {
				out = append(out, Chunk{Body: part})
			}
			continue
		}
		sep := 0
		if bufRunes > 0 {
			sep = 2 // "\n\n"
		}
		if bufRunes > 0 && bufRunes+sep+pr > maxRunes {
			flush()
			sep = 0
		}
		if sep > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(p)
		bufRunes = utf8.RuneCountInString(buf.String())
	}
	flush()
	return out
}

func hardSplitRunes(s string, maxRunes int) []string {
	runes := []rune(s)
	var parts []string
	for len(runes) > 0 {
		n := maxRunes
		if n > len(runes) {
			n = len(runes)
		}
		parts = append(parts, string(runes[:n]))
		runes = runes[n:]
	}
	return parts
}
