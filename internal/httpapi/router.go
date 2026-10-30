package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/account"
	"sharedmemory/internal/agents"
	"sharedmemory/internal/artifact"
	"sharedmemory/internal/attachment"
	"sharedmemory/internal/audit"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/device"
	"sharedmemory/internal/entity"
	"sharedmemory/internal/index"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/quarantine"
	"sharedmemory/internal/ratelimit"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/session"
	"sharedmemory/internal/vault"
)

type Deps struct {
	Pool        *pgxpool.Pool
	Auth        *auth.Store
	Projects    *project.Store
	Policies    *policy.Store
	Sessions    *session.Service
	Attachments *attachment.Service
	Memories    *memory.Store
	Recall      *recall.Service
	Vault       *vault.Store
	Artifacts   *artifact.Store
	Audit       *audit.Log
	Deleter     *account.Deleter
	Index       *index.Service
	Devices     *device.Store
	Quarantine  *quarantine.Store
	Entities    *entity.Store
	KB          *kb.Store
	Agents      *agents.Store
	Embedding     EmbeddingSettings
	EmbedResolver *semantic.Resolver
	EmbedMaxDim   int
	Reindexer     AccountReindexer
	RateLimit   *ratelimit.Limiter
	CORSOrigins []string
	// PublicAPIURL is baked into Cursor setup scripts (external host:port).
	PublicAPIURL string
	// DownloadsDir serves smhook.exe / shared-memory-mcp.exe at GET /downloads/{name}.
	DownloadsDir string
	// WebStaticDir optionally serves web/dist for same-origin UI.
	WebStaticDir string
}

type Router struct {
	Deps
}

func NewRouter(d Deps) http.Handler {
	r := &Router{Deps: d}
	mux := http.NewServeMux()
	authed := BearerAuth(d.Auth)
	handle := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, authed(h))
	}

	handleRL := func(pattern, route string, h http.HandlerFunc) {
		mux.Handle(pattern, authed(RateLimit(d.RateLimit, route)(h)))
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /downloads/{name}", r.handlePublicDownload)
	mux.HandleFunc("POST /v1/auth/register", r.handleRegister)
	mux.HandleFunc("POST /v1/auth/login", r.handleLogin)
	mux.HandleFunc("POST /v1/device/code", r.handleDeviceCode)
	mux.HandleFunc("POST /v1/device/token", r.handleDeviceToken)
	mux.HandleFunc("GET /v1/ws", r.handleWS)

	handle("GET /v1/me", r.handleMe)
	handle("POST /v1/auth/logout", r.handleLogout)
	handle("POST /v1/auth/api-keys", r.handleCreateAPIKey)
	handle("GET /v1/auth/api-keys", r.handleListAPIKeys)
	handle("DELETE /v1/auth/api-keys/{id}", r.handleRevokeAPIKey)
	handle("POST /v1/device/approve", r.handleDeviceApprove)
	handle("GET /v1/devices", r.handleListDevices)
	handle("DELETE /v1/devices/{id}", r.handleRevokeDevice)
	handle("POST /v1/devices/heartbeat", r.handleDeviceHeartbeat)

	handle("POST /v1/projects", r.handleCreateProject)
	handle("GET /v1/projects", r.handleListProjects)
	handle("DELETE /v1/projects/{id}", r.handleDeleteProject)
	handle("GET /v1/policy", r.handleGetPolicy)
	handle("PUT /v1/policy", r.handlePutPolicy)
	handle("GET /v1/audit", r.handleListAudit)
	handle("DELETE /v1/account", r.handleDeleteAccount)

	handleRL("POST /v1/ingest", "ingest", r.handleIngest)
	handle("GET /v1/sessions", r.handleListSessions)
	handle("GET /v1/sessions/{id}", r.handleGetSession)
	handle("POST /v1/attachments", r.handleUploadAttachment)
	handle("GET /v1/attachments/{id}", r.handleGetAttachment)
	handle("GET /v1/attachments/{id}/thumbnail", r.handleGetAttachmentThumb)
	handle("GET /v1/quarantine", r.handleListQuarantine)
	handle("POST /v1/quarantine/{id}/replay", r.handleReplayQuarantine)
	handle("GET /v1/entities", r.handleListEntities)
	handle("GET /v1/provenance/stats", r.handleProvenanceStats)

	handle("GET /v1/agents", r.handleListAgents)
	handle("GET /v1/agents/stream", r.handleAgentsStream)
	handle("POST /v1/agents/connections", r.handleCreateAgentConnection)
	handle("POST /v1/agents/connections/heartbeat", r.handleAgentsHeartbeat)
	handle("PATCH /v1/agents/connections/{id}", r.handlePatchAgentConnection)
	handle("DELETE /v1/agents/connections/{id}", r.handleDeleteAgentConnection)
	handle("POST /v1/agents/connections/{id}/upgrade", r.handleUpgradeAgentConnection)
	handle("POST /v1/agents/connections/{id}/claim-history", r.handleClaimAgentHistory)
	handle("GET /v1/agents/connections/{id}/setup-script", r.handleDownloadConnectionSetupScript)
	handle("GET /v1/agents/connections/{id}/setup-credentials", r.handleConnectionSetupCredentials)

	handleRL("POST /v1/recall", "recall", r.handleRecall)
	handle("GET /v1/recall/receipts", r.handleListRecallReceipts)
	handle("GET /v1/recall/receipts/check", r.handleCheckRecallReceipt)
	handle("GET /v1/memories", r.handleListMemories)
	handle("PUT /v1/memories/{id}", r.handleSupersedeMemory)
	handle("DELETE /v1/memories/{id}", r.handleInvalidateMemory)
	handle("GET /v1/profile", r.handleGetProfile)

	handle("GET /v1/candidates", r.handleListCandidates)
	handle("GET /v1/vault/candidates", r.handleListVaultCandidates)
	handle("POST /v1/vault/candidates/{id}/accept", r.handleAcceptVault)
	handle("POST /v1/vault/candidates/{id}/reject", r.handleRejectVault)
	handle("GET /v1/vault/items", r.handleListVault)
	handle("POST /v1/vault/items", r.handleCreateVault)
	handle("PUT /v1/vault/items/{id}", r.handleUpdateVault)
	handle("DELETE /v1/vault/items/{id}", r.handleDeleteVault)
	handle("POST /v1/vault/items/{id}/reveal", r.handleRevealVault)
	handle("POST /v1/vault/items/{id}/copied", r.handleCopiedVault)

	handle("GET /v1/artifacts/candidates", r.handleListArtifactCandidates)
	handle("POST /v1/artifacts/candidates/{id}/accept", r.handleAcceptArtifact)
	handle("POST /v1/artifacts/candidates/{id}/reject", r.handleRejectArtifact)
	handle("GET /v1/artifacts", r.handleListArtifacts)
	handle("POST /v1/artifacts", r.handleCreateArtifact)
	handle("GET /v1/artifacts/{id}", r.handleGetArtifact)
	handle("GET /v1/artifacts/{id}/export", r.handleExportArtifact)
	handle("PUT /v1/artifacts/{id}", r.handleUpdateArtifact)
	handle("DELETE /v1/artifacts/{id}", r.handleDeleteArtifact)

	handle("POST /v1/kb/documents", r.handleCreateKBDocument)
	handle("GET /v1/kb/documents", r.handleListKBDocuments)
	handle("GET /v1/kb/documents/{id}", r.handleGetKBDocument)
	handle("DELETE /v1/kb/documents/{id}", r.handleDeleteKBDocument)
	handle("POST /v1/kb/documents/{id}/reparse", r.handleReparseKBDocument)

	handle("GET /v1/embedding-settings", r.handleGetEmbeddingSettings)
	handle("PUT /v1/embedding-settings", r.handlePutEmbeddingSettings)
	handle("DELETE /v1/embedding-settings", r.handleDeleteEmbeddingSettings)
	handle("POST /v1/embedding-settings/probe", r.handleProbeEmbeddingSettings)
	handle("POST /v1/embedding-settings/reindex", r.handleReindexEmbeddingSettings)

	return withWebStatic(d.WebStaticDir, CORS(d.CORSOrigins)(mux))
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// fail maps domain errors to HTTP statuses; unknown errors are logged and hidden.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, session.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, session.ErrNotFound), errors.Is(err, project.ErrNotFound),
		errors.Is(err, vault.ErrNotFound), errors.Is(err, artifact.ErrNotFound),
		errors.Is(err, memory.ErrNotFound), errors.Is(err, attachment.ErrNotFound),
		errors.Is(err, account.ErrNotFound), errors.Is(err, kb.ErrNotFound),
		errors.Is(err, agents.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, attachment.ErrUnsupportedType), errors.Is(err, kb.ErrUnsupportedType):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, attachment.ErrTooLarge), errors.Is(err, kb.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, attachment.ErrNotStored):
		writeError(w, http.StatusGone, err.Error())
	case errors.Is(err, auth.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, req *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 8<<20))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return false
	}
	return true
}

func accountOf(req *http.Request) uuid.UUID {
	id, _ := AccountIDFromContext(req.Context())
	return id
}

func pathID(w http.ResponseWriter, req *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(req.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return uuid.Nil, false
	}
	return id, true
}

// optionalUUID parses an optional uuid query/body value; "" means absent.
func optionalUUID(w http.ResponseWriter, raw, field string) (*uuid.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, field+" must be a uuid")
		return nil, false
	}
	return &id, true
}

func (r *Router) audit(req *http.Request, action string, meta map[string]any) {
	if r.Audit == nil {
		return
	}
	if err := r.Audit.Record(req.Context(), accountOf(req), action, meta); err != nil {
		log.Printf("audit %s: %v", action, err)
	}
}
