package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/auth"
)

type Router struct {
	auth *auth.Store
}

func NewRouter(pool *pgxpool.Pool) http.Handler {
	r := &Router{auth: auth.NewStore(pool)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /v1/auth/register", r.handleRegister)
	mux.Handle("POST /v1/auth/api-keys", BearerAuth(r.auth)(http.HandlerFunc(r.handleCreateAPIKey)))
	return mux
}

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerResp struct {
	AccountID string `json:"account_id"`
	APIKey    string `json:"api_key"`
}

func (r *Router) handleRegister(w http.ResponseWriter, req *http.Request) {
	var body registerReq
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	id, key, err := r.auth.Register(req.Context(), body.Email, body.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, registerResp{
		AccountID: id.String(),
		APIKey:    key,
	})
}

type createKeyReq struct {
	Name string `json:"name"`
}

type createKeyResp struct {
	APIKey string `json:"api_key"`
}

func (r *Router) handleCreateAPIKey(w http.ResponseWriter, req *http.Request) {
	accountID, ok := AccountIDFromContext(req.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body createKeyReq
	_ = json.NewDecoder(req.Body).Decode(&body)
	if body.Name == "" {
		body.Name = "additional"
	}
	key, err := r.auth.CreateAPIKey(req.Context(), accountID, body.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, createKeyResp{APIKey: key})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
