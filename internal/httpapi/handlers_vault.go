package httpapi

import (
	"net/http"

	"sharedmemory/internal/audit"
	"sharedmemory/internal/vault"
)

func (r *Router) handleListCandidates(w http.ResponseWriter, req *http.Request) {
	vc, err := r.Vault.ListCandidates(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	ac, err := r.Artifacts.ListCandidates(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"vault": vc, "artifacts": ac})
}

func (r *Router) handleListVaultCandidates(w http.ResponseWriter, req *http.Request) {
	list, err := r.Vault.ListCandidates(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleAcceptVault(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var o vault.AcceptOverrides
	if req.ContentLength != 0 && !decode(w, req, &o) {
		return
	}
	it, err := r.Vault.Accept(req.Context(), accountOf(req), id, o)
	if err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.VaultAccept, map[string]any{"candidate_id": id, "item_id": it.ID, "title": it.Title})
	writeJSON(w, http.StatusCreated, it)
}

func (r *Router) handleRejectVault(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Vault.Reject(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.VaultReject, map[string]any{"candidate_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleListVault(w http.ResponseWriter, req *http.Request) {
	list, err := r.Vault.List(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleCreateVault(w http.ResponseWriter, req *http.Request) {
	var in vault.ItemInput
	if !decode(w, req, &in) {
		return
	}
	if in.ProjectID != nil {
		if _, err := r.Projects.Get(req.Context(), accountOf(req), *in.ProjectID); err != nil {
			fail(w, err)
			return
		}
	}
	it, err := r.Vault.Create(req.Context(), accountOf(req), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	r.audit(req, audit.VaultCreate, map[string]any{"item_id": it.ID, "title": it.Title})
	writeJSON(w, http.StatusCreated, it)
}

func (r *Router) handleUpdateVault(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var in vault.ItemInput
	if !decode(w, req, &in) {
		return
	}
	if err := r.Vault.Update(req.Context(), accountOf(req), id, in); err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.VaultUpdate, map[string]any{"item_id": id, "secret_changed": in.Secret != ""})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleDeleteVault(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Vault.Delete(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.VaultDelete, map[string]any{"item_id": id})
	w.WriteHeader(http.StatusNoContent)
}

type revealReq struct {
	Password string `json:"password"`
}

// handleRevealVault requires the account password on every reveal, even with
// a valid API key, and audits both successful and failed attempts.
func (r *Router) handleRevealVault(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var body revealReq
	if !decode(w, req, &body) {
		return
	}
	if err := r.Auth.VerifyPassword(req.Context(), accountOf(req), body.Password); err != nil {
		r.audit(req, audit.VaultReveal, map[string]any{"item_id": id, "ok": false})
		writeError(w, http.StatusUnauthorized, "password verification failed")
		return
	}
	secret, err := r.Vault.RevealWithPassword(req.Context(), accountOf(req), id, body.Password)
	if err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.VaultReveal, map[string]any{"item_id": id, "ok": true})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
}

func (r *Router) handleVaultWrapStatus(w http.ResponseWriter, req *http.Request) {
	ok, err := r.Vault.PasswordWrapConfigured(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password_wrap_enabled": ok})
}

func (r *Router) handleVaultWrapSetup(w http.ResponseWriter, req *http.Request) {
	var body revealReq
	if !decode(w, req, &body) {
		return
	}
	if err := r.Auth.VerifyPassword(req.Context(), accountOf(req), body.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "password verification failed")
		return
	}
	accountID := accountOf(req)
	wasWrapped, err := r.Vault.PasswordWrapConfigured(req.Context(), accountID)
	if err != nil {
		fail(w, err)
		return
	}
	dek, err := r.Vault.EnsurePasswordDEK(req.Context(), accountID, body.Password)
	if err != nil {
		fail(w, err)
		return
	}
	if !wasWrapped {
		if err := r.Vault.ReencryptSecretsWithDEK(req.Context(), accountID, dek); err != nil {
			fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"password_wrap_enabled": true})
}

// handleCopiedVault records that a revealed secret was copied to the clipboard.
func (r *Router) handleCopiedVault(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Vault.Exists(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.VaultCopy, map[string]any{"item_id": id})
	w.WriteHeader(http.StatusNoContent)
}
