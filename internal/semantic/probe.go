package semantic

import (
	"context"
	"fmt"
)

// Probe embeds a short probe string and returns the vector dimension for the given protocol.
func Probe(ctx context.Context, protocol, baseURL, model, apiKey string) (int, error) {
	switch protocol {
	case "tei":
		e := NewTEIEmbedder(baseURL, 0)
		vec, err := e.Embed(ctx, "probe")
		if err != nil {
			return 0, err
		}
		return len(vec), nil
	case "openai":
		e := NewOpenAIEmbedder(baseURL, apiKey, model, 0)
		vec, err := e.Embed(ctx, "probe text")
		if err != nil {
			return 0, err
		}
		return len(vec), nil
	default:
		return 0, fmt.Errorf("unsupported protocol %q", protocol)
	}
}
