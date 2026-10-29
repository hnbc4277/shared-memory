package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"sharedmemory/internal/semantic"
)

// EmbeddingSettings is the persistence layer for account embedding overrides.
type EmbeddingSettings interface {
	Get(ctx context.Context, accountID uuid.UUID) (semantic.Settings, error)
	Upsert(ctx context.Context, accountID uuid.UUID, st semantic.Settings, apiKeyPlain ...string) error
	Clear(ctx context.Context, accountID uuid.UUID) error
	PlainAPIKey(ctx context.Context, accountID uuid.UUID) (string, error)
}

// AccountReindexer rebuilds vector indexes for one account (Task 6).
type AccountReindexer interface {
	RunAccount(ctx context.Context, accountID uuid.UUID) (int, error)
}

type embeddingDeploymentJSON struct {
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
	Dim      int    `json:"dim"`
	URLSet   bool   `json:"url_set"`
}

type embeddingSettingsJSON struct {
	Source          string                  `json:"source"`
	Enabled         bool                    `json:"enabled"`
	Protocol        string                  `json:"protocol"`
	BaseURL         string                  `json:"base_url"`
	Model           string                  `json:"model"`
	Dim             int                     `json:"dim"`
	APIKeySet       bool                    `json:"api_key_set"`
	ReindexRequired bool                    `json:"reindex_required"`
	MaxDim          int                     `json:"max_dim"`
	Deployment      embeddingDeploymentJSON `json:"deployment"`
}

type putEmbeddingBody struct {
	Enabled  bool    `json:"enabled"`
	Protocol string  `json:"protocol"`
	BaseURL  string  `json:"base_url"`
	Model    string  `json:"model"`
	APIKey   *string `json:"api_key"`
}

type probeEmbeddingBody struct {
	Protocol string  `json:"protocol"`
	BaseURL  string  `json:"base_url"`
	Model    string  `json:"model"`
	APIKey   *string `json:"api_key"`
}

func (r *Router) handleGetEmbeddingSettings(w http.ResponseWriter, req *http.Request) {
	accountID, ok := requireAccountID(w, req)
	if !ok {
		return
	}
	if r.Embedding == nil {
		writeError(w, http.StatusNotImplemented, "embedding settings not configured")
		return
	}
	st, err := r.Embedding.Get(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, r.buildEmbeddingSettingsJSON(req.Context(), accountID, st))
}

func (r *Router) handlePutEmbeddingSettings(w http.ResponseWriter, req *http.Request) {
	accountID, ok := requireAccountID(w, req)
	if !ok {
		return
	}
	if r.Embedding == nil {
		writeError(w, http.StatusNotImplemented, "embedding settings not configured")
		return
	}
	var body putEmbeddingBody
	if !decode(w, req, &body) {
		return
	}
	protocol := strings.TrimSpace(body.Protocol)
	if protocol != "tei" && protocol != "openai" {
		writeError(w, http.StatusBadRequest, `protocol must be "tei" or "openai"`)
		return
	}
	baseURL, err := normalizeEmbeddingBaseURL(body.BaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	model := strings.TrimSpace(body.Model)
	if protocol == "openai" && model == "" {
		writeError(w, http.StatusBadRequest, "model is required for openai protocol")
		return
	}
	apiKey := resolveProbeAPIKey(req.Context(), r.Embedding, accountID, body.APIKey)
	dim, err := semantic.Probe(req.Context(), protocol, baseURL, model, apiKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.EmbedMaxDim > 0 && dim > r.EmbedMaxDim {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("embedding dim %d exceeds max %d", dim, r.EmbedMaxDim))
		return
	}
	existing, err := r.Embedding.Get(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	newFP := semantic.Fingerprint(protocol, model, dim, baseURL)
	reindexRequired := newFP != existing.ActiveFingerprint
	st := semantic.Settings{
		Enabled:           body.Enabled,
		Protocol:          protocol,
		BaseURL:           baseURL,
		Model:             model,
		Dim:               dim,
		ActiveFingerprint: existing.ActiveFingerprint,
		ReindexRequired:   reindexRequired,
	}
	if err := upsertEmbeddingSettings(req.Context(), r.Embedding, accountID, st, body.APIKey); err != nil {
		fail(w, err)
		return
	}
	got, err := r.Embedding.Get(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, r.buildEmbeddingSettingsJSON(req.Context(), accountID, got))
}

func (r *Router) handleDeleteEmbeddingSettings(w http.ResponseWriter, req *http.Request) {
	accountID, ok := requireAccountID(w, req)
	if !ok {
		return
	}
	if r.Embedding == nil {
		writeError(w, http.StatusNotImplemented, "embedding settings not configured")
		return
	}
	existing, err := r.Embedding.Get(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	if !embeddingRowPresent(existing) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	deployFP := deploymentFingerprint(r.EmbedResolver)
	if existing.ActiveFingerprint == "" || existing.ActiveFingerprint == deployFP {
		if err := r.Embedding.Clear(req.Context(), accountID); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, r.buildEmbeddingSettingsJSON(req.Context(), accountID, semantic.Settings{}))
		return
	}
	st := semantic.Settings{
		Enabled:           false,
		Protocol:          existing.Protocol,
		BaseURL:           "",
		Model:             "",
		Dim:               0,
		ActiveFingerprint: existing.ActiveFingerprint,
		ReindexRequired:   true,
	}
	if err := r.Embedding.Upsert(req.Context(), accountID, st, ""); err != nil {
		fail(w, err)
		return
	}
	got, err := r.Embedding.Get(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, r.buildEmbeddingSettingsJSON(req.Context(), accountID, got))
}

func (r *Router) handleProbeEmbeddingSettings(w http.ResponseWriter, req *http.Request) {
	accountID, ok := requireAccountID(w, req)
	if !ok {
		return
	}
	if r.Embedding == nil {
		writeError(w, http.StatusNotImplemented, "embedding settings not configured")
		return
	}
	var body probeEmbeddingBody
	if !decode(w, req, &body) {
		return
	}
	protocol := strings.TrimSpace(body.Protocol)
	if protocol != "tei" && protocol != "openai" {
		writeError(w, http.StatusBadRequest, `protocol must be "tei" or "openai"`)
		return
	}
	baseURL, err := normalizeEmbeddingBaseURL(body.BaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	model := strings.TrimSpace(body.Model)
	if protocol == "openai" && model == "" {
		writeError(w, http.StatusBadRequest, "model is required for openai protocol")
		return
	}
	apiKey := resolveProbeAPIKey(req.Context(), r.Embedding, accountID, body.APIKey)
	dim, err := semantic.Probe(req.Context(), protocol, baseURL, model, apiKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.EmbedMaxDim > 0 && dim > r.EmbedMaxDim {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("embedding dim %d exceeds max %d", dim, r.EmbedMaxDim))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"dim": dim})
}

func (r *Router) handleReindexEmbeddingSettings(w http.ResponseWriter, req *http.Request) {
	accountID, ok := requireAccountID(w, req)
	if !ok {
		return
	}
	if r.Reindexer == nil {
		writeError(w, http.StatusNotImplemented, "reindex not implemented")
		return
	}
	n, err := r.Reindexer.RunAccount(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

func requireAccountID(w http.ResponseWriter, req *http.Request) (uuid.UUID, bool) {
	id, ok := AccountIDFromContext(req.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return id, true
}

func normalizeEmbeddingBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return "", fmt.Errorf("base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base_url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base_url must use http or https")
	}
	return raw, nil
}

func resolveProbeAPIKey(ctx context.Context, store EmbeddingSettings, accountID uuid.UUID, apiKey *string) string {
	if apiKey != nil && strings.TrimSpace(*apiKey) != "" {
		return *apiKey
	}
	if apiKey != nil && *apiKey == "" {
		return ""
	}
	key, err := store.PlainAPIKey(ctx, accountID)
	if err != nil {
		return ""
	}
	return key
}

func upsertEmbeddingSettings(ctx context.Context, store EmbeddingSettings, accountID uuid.UUID, st semantic.Settings, apiKey *string) error {
	if apiKey == nil {
		return store.Upsert(ctx, accountID, st)
	}
	return store.Upsert(ctx, accountID, st, *apiKey)
}

func deploymentFingerprint(res *semantic.Resolver) string {
	if res == nil || strings.TrimSpace(res.DeployURL) == "" {
		return ""
	}
	dim := res.DeployDim
	if dim <= 0 {
		dim = 512
	}
	return semantic.Fingerprint(res.DeployProtocol, res.DeployModel, dim, res.DeployURL)
}

func embeddingRowPresent(st semantic.Settings) bool {
	return st.Enabled || st.Protocol != "" || st.BaseURL != "" || st.Model != "" || st.Dim > 0 ||
		st.ActiveFingerprint != "" || st.ReindexRequired || st.APIKeySet
}

func (r *Router) buildEmbeddingSettingsJSON(ctx context.Context, accountID uuid.UUID, st semantic.Settings) embeddingSettingsJSON {
	dep := embeddingDeploymentJSON{
		Protocol: "tei",
		Model:    "bge-small-zh-v1.5",
		Dim:      512,
	}
	source := "hash"
	reindexRequired := st.ReindexRequired
	if r.EmbedResolver != nil {
		if r.EmbedResolver.DeployProtocol != "" {
			dep.Protocol = r.EmbedResolver.DeployProtocol
		}
		if r.EmbedResolver.DeployModel != "" {
			dep.Model = r.EmbedResolver.DeployModel
		}
		if r.EmbedResolver.DeployDim > 0 {
			dep.Dim = r.EmbedResolver.DeployDim
		}
		dep.URLSet = strings.TrimSpace(r.EmbedResolver.DeployURL) != ""
		res, err := r.EmbedResolver.ForAccount(ctx, accountID)
		if err == nil {
			source = res.Source
			reindexRequired = res.ReindexRequired
		}
	}
	maxDim := r.EmbedMaxDim
	if maxDim <= 0 {
		maxDim = 4096
	}
	return embeddingSettingsJSON{
		Source:          source,
		Enabled:         st.Enabled,
		Protocol:        st.Protocol,
		BaseURL:         st.BaseURL,
		Model:           st.Model,
		Dim:             st.Dim,
		APIKeySet:       st.APIKeySet,
		ReindexRequired: reindexRequired,
		MaxDim:          maxDim,
		Deployment:      dep,
	}
}
