package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Settings mirrors account_embedding_settings columns exposed to callers.
// API key material stays encrypted in the store; APIKeySet indicates a stored key.
type Settings struct {
	Enabled           bool
	Protocol          string
	BaseURL           string
	Model             string
	Dim               int
	ActiveFingerprint string
	ReindexRequired   bool
	APIKeySet         bool
}

func normalizeBaseURL(u string) string {
	u = strings.TrimSpace(u)
	return strings.TrimRight(u, "/")
}

// Fingerprint identifies an embedder configuration for reindex tracking
// (protocol|model|dim|url_hash).
func Fingerprint(protocol, model string, dim int, baseURL string) string {
	sum := sha256.Sum256([]byte(normalizeBaseURL(baseURL)))
	urlHash := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s|%s|%d|%s", protocol, model, dim, urlHash)
}
