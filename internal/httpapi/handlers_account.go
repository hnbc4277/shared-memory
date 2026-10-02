package httpapi

import (
	"log"
	"net/http"
	"strconv"

	"sharedmemory/internal/audit"
)

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
	if !decode(w, req, &body) {
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	id, key, err := r.Auth.Register(req.Context(), body.Email, body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, registerResp{AccountID: id.String(), APIKey: key})
}

func (r *Router) handleMe(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"account_id": accountOf(req).String()})
}

type createKeyReq struct {
	Name string `json:"name"`
}

func (r *Router) handleCreateAPIKey(w http.ResponseWriter, req *http.Request) {
	var body createKeyReq
	if req.ContentLength != 0 && !decode(w, req, &body) {
		return
	}
	if body.Name == "" {
		body.Name = "additional"
	}
	key, err := r.Auth.CreateAPIKey(req.Context(), accountOf(req), body.Name)
	if err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.APIKeyCreate, map[string]any{"name": body.Name})
	writeJSON(w, http.StatusCreated, map[string]string{"api_key": key})
}

func (r *Router) handleListAPIKeys(w http.ResponseWriter, req *http.Request) {
	keys, err := r.Auth.ListAPIKeys(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (r *Router) handleRevokeAPIKey(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Auth.RevokeAPIKey(req.Context(), accountOf(req), id); err != nil {
		writeError(w, http.StatusNotFound, "api key not found")
		return
	}
	r.audit(req, audit.APIKeyRevoke, map[string]any{"key_id": id})
	w.WriteHeader(http.StatusNoContent)
}

type createProjectReq struct {
	Name string `json:"name"`
}

func (r *Router) handleCreateProject(w http.ResponseWriter, req *http.Request) {
	var body createProjectReq
	if !decode(w, req, &body) {
		return
	}
	p, err := r.Projects.Create(req.Context(), accountOf(req), body.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (r *Router) handleListProjects(w http.ResponseWriter, req *http.Request) {
	list, err := r.Projects.List(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleDeleteProject(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Deleter.DeleteProject(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	if _, err := r.Memories.RebuildProfile(req.Context(), accountOf(req)); err != nil {
		log.Printf("rebuild profile: %v", err)
	}
	r.audit(req, audit.ProjectDelete, map[string]any{"project_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleGetPolicy(w http.ResponseWriter, req *http.Request) {
	p, err := r.Policies.Get(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// policyPatch lets clients send only the toggles they change.
type policyPatch struct {
	StoreTranscript    *bool `json:"store_transcript"`
	StoreAttachments   *bool `json:"store_attachments"`
	InjectEnabled      *bool `json:"inject_enabled"`
	VaultEnabled       *bool `json:"vault_enabled"`
	ArtifactSearchable *bool `json:"artifact_searchable"`
	RetentionDays      *int  `json:"retention_days"`
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func (r *Router) handlePutPolicy(w http.ResponseWriter, req *http.Request) {
	var patch policyPatch
	if !decode(w, req, &patch) {
		return
	}
	p, err := r.Policies.Get(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	setIf(&p.StoreTranscript, patch.StoreTranscript)
	setIf(&p.StoreAttachments, patch.StoreAttachments)
	setIf(&p.InjectEnabled, patch.InjectEnabled)
	setIf(&p.VaultEnabled, patch.VaultEnabled)
	setIf(&p.ArtifactSearchable, patch.ArtifactSearchable)
	setIf(&p.RetentionDays, patch.RetentionDays)
	if err := r.Policies.Update(req.Context(), accountOf(req), p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	r.audit(req, audit.PolicyUpdate, map[string]any{"policy": p})
	writeJSON(w, http.StatusOK, p)
}

func (r *Router) handleListAudit(w http.ResponseWriter, req *http.Request) {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	list, err := r.Audit.List(req.Context(), accountOf(req), limit)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleDeleteAccount(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("X-Confirm") != "delete" {
		writeError(w, http.StatusPreconditionRequired, "set header X-Confirm: delete to permanently delete this account")
		return
	}
	if err := r.Deleter.DeleteAccount(req.Context(), accountOf(req)); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
