package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"sharedmemory/internal/attachment"
	"sharedmemory/internal/quarantine"
	"sharedmemory/internal/session"
)

type provenanceStat struct {
	Agent            string  `json:"agent"`
	Messages         int     `json:"messages"`
	UnknownModelPct  float64 `json:"unknown_model_pct"`
	EmptySkillsPct   float64 `json:"empty_skills_pct"`
}

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

func (r *Router) handleReplayQuarantine(w http.ResponseWriter, req *http.Request) {
	if r.Quarantine == nil || r.Sessions == nil {
		writeError(w, http.StatusNotImplemented, "quarantine replay unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	accountID := accountOf(req)
	entry, err := r.Quarantine.Get(req.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, quarantine.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		fail(w, err)
		return
	}
	var body session.IngestMessageRequest
	raw, _ := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	trimmed := bytes.TrimSpace(raw)
	// Clients often POST "{}"; treat empty / null / {} as "use stored payload".
	useStored := len(trimmed) == 0 || string(trimmed) == "{}" || string(trimmed) == "null"
	if useStored {
		if err := json.Unmarshal(entry.Payload, &body); err != nil {
			writeError(w, http.StatusBadRequest, "stored payload is not a valid ingest body")
			return
		}
	} else if err := json.Unmarshal(trimmed, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	res, err := r.Sessions.Ingest(req.Context(), accountID, body)
	if err != nil {
		if errors.Is(err, session.ErrInvalid) {
			_ = r.Quarantine.UpdateReason(req.Context(), accountID, id, err.Error())
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		fail(w, err)
		return
	}
	_ = r.Quarantine.Delete(req.Context(), accountID, id)
	writeJSON(w, http.StatusOK, map[string]any{
		"replayed":   true,
		"session_id": res.SessionID,
		"message_id": res.MessageID,
	})
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

func (r *Router) handleProvenanceStats(w http.ResponseWriter, req *http.Request) {
	rows, err := r.Pool.Query(req.Context(), `
		SELECT COALESCE(agent, ''),
		       COUNT(*)::int,
		       AVG(CASE WHEN COALESCE(model, '') IN ('', 'unknown') THEN 1.0 ELSE 0.0 END)::float8,
		       AVG(CASE WHEN COALESCE(skills, '[]'::jsonb) = '[]'::jsonb THEN 1.0 ELSE 0.0 END)::float8
		FROM messages
		WHERE account_id = $1 AND created_at > now() - interval '7 days'
		GROUP BY agent
		ORDER BY COUNT(*) DESC`, accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []provenanceStat{}
	for rows.Next() {
		var s provenanceStat
		if err := rows.Scan(&s.Agent, &s.Messages, &s.UnknownModelPct, &s.EmptySkillsPct); err != nil {
			fail(w, err)
			return
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"by_agent": out})
}
