package semantic

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// AccountSettings loads per-account embedding configuration without exposing API keys via Get.
type AccountSettings interface {
	Get(ctx context.Context, accountID uuid.UUID) (Settings, error)
	PlainAPIKey(ctx context.Context, accountID uuid.UUID) (string, error)
}

type Resolved struct {
	Source          string // "account" | "deployment" | "hash"
	Embedder        Embedder
	Protocol        string
	Model           string
	Dim             int
	BaseURL         string
	Collection      string
	ReindexRequired bool
}

type Resolver struct {
	Settings AccountSettings

	DeployProtocol string
	DeployURL      string
	DeployModel    string
	DeployAPIKey   string
	DeployDim      int

	MaxDim int
}

var _ AccountSettings = (*Store)(nil)

func (r *Resolver) ForAccount(ctx context.Context, accountID uuid.UUID) (Resolved, error) {
	st, err := r.Settings.Get(ctx, accountID)
	if err != nil {
		return Resolved{}, err
	}

	if accountSettingsValid(st) {
		if r.MaxDim > 0 && st.Dim > r.MaxDim {
			return Resolved{}, fmt.Errorf("embedding dim %d exceeds max %d", st.Dim, r.MaxDim)
		}
		apiKey := ""
		if st.Protocol == "openai" {
			apiKey, err = r.Settings.PlainAPIKey(ctx, accountID)
			if err != nil {
				return Resolved{}, err
			}
		}
		emb, err := newEmbedder(st.Protocol, st.BaseURL, apiKey, st.Model, st.Dim)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{
			Source:          "account",
			Embedder:        emb,
			Protocol:        st.Protocol,
			Model:           st.Model,
			Dim:             st.Dim,
			BaseURL:         st.BaseURL,
			Collection:      CollectionName(accountID, st.Model, st.Dim),
			ReindexRequired: st.ReindexRequired,
		}, nil
	}

	if strings.TrimSpace(r.DeployURL) != "" {
		dim := r.DeployDim
		if dim <= 0 {
			dim = 512
		}
		emb, err := newEmbedder(r.DeployProtocol, r.DeployURL, r.DeployAPIKey, r.DeployModel, dim)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{
			Source:     "deployment",
			Embedder:   emb,
			Protocol:   r.DeployProtocol,
			Model:      r.DeployModel,
			Dim:        dim,
			BaseURL:    r.DeployURL,
			Collection: CollectionName(accountID, r.DeployModel, dim),
		}, nil
	}

	return Resolved{
		Source:   "hash",
		Embedder: HashEmbedder{},
		Protocol: "hash",
		Dim:      HashDim,
	}, nil
}

func accountSettingsValid(st Settings) bool {
	if !st.Enabled || strings.TrimSpace(st.BaseURL) == "" || st.Dim <= 0 {
		return false
	}
	if st.Protocol == "openai" && strings.TrimSpace(st.Model) == "" {
		return false
	}
	return true
}

func newEmbedder(protocol, baseURL, apiKey, model string, dim int) (Embedder, error) {
	switch protocol {
	case "openai":
		return NewOpenAIEmbedder(baseURL, apiKey, model, dim), nil
	default:
		return NewTEIEmbedder(baseURL, dim), nil
	}
}

// CollectionName builds a per-account Qdrant collection: sm_memories_{compact}_{slug}_{dim}.
func CollectionName(accountID uuid.UUID, model string, dim int) string {
	hex := strings.ReplaceAll(accountID.String(), "-", "")
	compact := hex
	if len(compact) > 12 {
		compact = compact[:12]
	}
	var slug strings.Builder
	for _, r := range strings.ToLower(model) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			slug.WriteRune(r)
		} else {
			slug.WriteByte('_')
		}
	}
	return fmt.Sprintf("sm_memories_%s_%s_%d", compact, slug.String(), dim)
}
