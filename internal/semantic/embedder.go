package semantic

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

const HashDim = 64

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dim() int
}

// HashEmbedder is a deterministic feature-hashing embedder: lowercase word
// tokens plus CJK character bigrams. It needs no model or network, so recall
// works offline; swap in a model-backed Embedder later without schema changes
// as long as Dim matches the Qdrant collection.
type HashEmbedder struct{}

func (HashEmbedder) Dim() int { return HashDim }

func (HashEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, HashDim)
	for _, tok := range Tokens(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		sum := h.Sum32()
		sign := float32(1)
		if sum&1 == 1 {
			sign = -1
		}
		vec[(sum>>1)%HashDim] += sign
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm == 0 {
		vec[0] = 1
		return vec, nil
	}
	n := float32(math.Sqrt(norm))
	for i := range vec {
		vec[i] /= n
	}
	return vec, nil
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

// Tokens splits text into latin words and CJK bigrams (unigram for lone chars).
func Tokens(text string) []string {
	var out []string
	var word []rune
	var cjk []rune
	flushWord := func() {
		if len(word) > 1 {
			out = append(out, strings.ToLower(string(word)))
		}
		word = word[:0]
	}
	flushCJK := func() {
		switch {
		case len(cjk) == 1:
			out = append(out, string(cjk))
		case len(cjk) > 1:
			for i := 0; i+1 < len(cjk); i++ {
				out = append(out, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range text {
		switch {
		case isCJK(r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return out
}
