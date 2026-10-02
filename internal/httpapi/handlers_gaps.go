package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"sharedmemory/internal/attachment"
)

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	var body loginReq
	if !decode(w, req, &body) {
		return
	}
	sess, err := r.Auth.LoginWithPassword(req.Context(), body.Email, body.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      sess.Token,
		"expires_at": sess.ExpiresAt,
		"account_id": sess.AccountID.String(),
	})
}

func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	h := req.Header.Get("Authorization")
	token := ""
	if len(h) > 7 {
		token = h[7:]
	}
	_ = r.Auth.RevokeWebSession(req.Context(), token)
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleGetAttachmentThumb(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	url, err := r.Attachments.ThumbnailURL(req.Context(), accountOf(req), id)
	if errors.Is(err, attachment.ErrNotStored) {
		writeJSON(w, http.StatusOK, map[string]any{"thumbnail_url": nil})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thumbnail_url": url})
}

func (r *Router) handleListQuarantine(w http.ResponseWriter, req *http.Request) {
	if r.Quarantine == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	list, err := r.Quarantine.List(req.Context(), accountOf(req), limit)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleListEntities(w http.ResponseWriter, req *http.Request) {
	if r.Entities == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	projectID, ok := optionalUUID(w, req.URL.Query().Get("project_id"), "project_id")
	if !ok {
		return
	}
	list, err := r.Entities.List(req.Context(), accountOf(req), projectID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
