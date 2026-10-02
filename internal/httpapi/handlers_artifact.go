package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/audit"
	"sharedmemory/internal/vault"
)

// indexArtifact projects an artifact into search when the account allows it.
func (r *Router) indexArtifact(req *http.Request, d artifact.Detail) {
	if r.Index == nil {
		return
	}
	ctx := req.Context()
	accountID := accountOf(req)
	pol, err := r.Policies.Get(ctx, accountID)
	if err != nil {
		log.Printf("artifact index policy: %v", err)
		return
	}
	if !pol.ArtifactSearchable {
		return
	}
	if err := r.Index.IndexDoc(ctx, artifact.SearchDoc(accountID, d)); err != nil {
		log.Printf("artifact index %s: %v", d.Artifact.ID, err)
	}
}

func (r *Router) handleListArtifactCandidates(w http.ResponseWriter, req *http.Request) {
	list, err := r.Artifacts.ListCandidates(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (r *Router) handleAcceptArtifact(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var o artifact.AcceptOverrides
	if req.ContentLength != 0 && !decode(w, req, &o) {
		return
	}
	d, err := r.Artifacts.Accept(req.Context(), accountOf(req), id, o)
	if err != nil {
		fail(w, err)
		return
	}
	r.indexArtifact(req, d)
	r.audit(req, audit.ArtifactAccept, map[string]any{"candidate_id": id, "artifact_id": d.Artifact.ID})
	writeJSON(w, http.StatusCreated, d)
}

func (r *Router) handleRejectArtifact(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Artifacts.Reject(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	r.audit(req, audit.ArtifactReject, map[string]any{"candidate_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleListArtifacts(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	projectID, ok := optionalUUID(w, q.Get("project_id"), "project_id")
	if !ok {
		return
	}
	list, err := r.Artifacts.List(req.Context(), accountOf(req), artifact.Filter{
		ProjectID: projectID, Type: q.Get("type"), Tag: q.Get("tag"),
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type createArtifactReq struct {
	artifact.Input
	FromMessageID     string `json:"from_message_id"`
	AcknowledgeSecret bool   `json:"acknowledge_secret"`
	// MoveSecretsToVault saves detected credentials as vault items and keeps
	// only the redacted body in the artifact.
	MoveSecretsToVault bool `json:"move_secrets_to_vault"`
}

type artifactRes struct {
	artifact.Detail
	MovedToVault int `json:"moved_to_vault,omitempty"`
}

func secretConflict(w http.ResponseWriter) {
	writeError(w, http.StatusConflict,
		"content looks like it contains a password/token; resend with move_secrets_to_vault=true to keep it in the vault, or acknowledge_secret=true to keep it in the artifact")
}

// moveSecretsToVault stores every credential found in body as a vault item
// (the user asked for exactly this) and returns the redacted body.
func (r *Router) moveSecretsToVault(req *http.Request, projectID *uuid.UUID, body string) (string, int, error) {
	n := 0
	for _, d := range vault.Detect(body) {
		it, err := r.Vault.Create(req.Context(), accountOf(req), vault.ItemInput{
			ProjectID: projectID, Title: d.Title, SiteURL: d.SiteURL, Username: d.Username, Secret: d.Secret})
		if err != nil {
			return "", n, err
		}
		r.audit(req, audit.VaultCreate, map[string]any{"item_id": it.ID, "title": it.Title, "from": "artifact"})
		n++
	}
	return vault.Redact(body), n, nil
}

func (r *Router) handleCreateArtifact(w http.ResponseWriter, req *http.Request) {
	var body createArtifactReq
	if !decode(w, req, &body) {
		return
	}
	ctx := req.Context()
	accountID := accountOf(req)
	src := artifact.Source{AgentPath: "user/manual"}
	if body.FromMessageID != "" {
		msgID, err := uuid.Parse(body.FromMessageID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from_message_id must be a uuid")
			return
		}
		m, err := r.Sessions.MessageContent(ctx, accountID, msgID)
		if err != nil {
			fail(w, err)
			return
		}
		if m.Content == nil {
			writeError(w, http.StatusConflict, "the transcript of this message was not stored; paste the body instead")
			return
		}
		if strings.TrimSpace(body.Body) == "" {
			body.Body = *m.Content
		}
		if body.ProjectID == nil {
			body.ProjectID = m.ProjectID
		}
		src = artifact.Source{ProjectID: m.ProjectID, SessionID: &m.SessionID, MessageID: &msgID, AgentPath: m.AgentPath}
	}
	if body.ProjectID != nil {
		if _, err := r.Projects.Get(ctx, accountID, *body.ProjectID); err != nil {
			fail(w, err)
			return
		}
	}
	moved := 0
	if body.MoveSecretsToVault {
		redacted, n, err := r.moveSecretsToVault(req, body.ProjectID, body.Body)
		if err != nil {
			fail(w, err)
			return
		}
		body.Body, moved = redacted, n
	} else if !body.AcknowledgeSecret && vault.ContainsSecret(body.Body) {
		secretConflict(w)
		return
	}
	src.ProjectID = body.ProjectID
	d, err := r.Artifacts.Create(ctx, accountID, body.Input, src)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	r.indexArtifact(req, d)
	r.audit(req, audit.ArtifactCreate, map[string]any{"artifact_id": d.Artifact.ID, "from_message_id": body.FromMessageID,
		"moved_to_vault": moved})
	writeJSON(w, http.StatusCreated, artifactRes{Detail: d, MovedToVault: moved})
}

func (r *Router) handleGetArtifact(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	d, err := r.Artifacts.Get(req.Context(), accountOf(req), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (r *Router) handleExportArtifact(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	d, err := r.Artifacts.Get(req.Context(), accountOf(req), id)
	if err != nil {
		fail(w, err)
		return
	}
	name := url.PathEscape(d.Artifact.Title) + ".md"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", name))
	_, _ = w.Write([]byte(d.Body))
}

type updateArtifactReq struct {
	artifact.Update
	AcknowledgeSecret  bool `json:"acknowledge_secret"`
	MoveSecretsToVault bool `json:"move_secrets_to_vault"`
}

func (r *Router) handleUpdateArtifact(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var body updateArtifactReq
	if !decode(w, req, &body) {
		return
	}
	moved := 0
	if body.Body != nil && body.MoveSecretsToVault {
		cur, err := r.Artifacts.Get(req.Context(), accountOf(req), id)
		if err != nil {
			fail(w, err)
			return
		}
		redacted, n, err := r.moveSecretsToVault(req, cur.Artifact.ProjectID, *body.Body)
		if err != nil {
			fail(w, err)
			return
		}
		body.Body, moved = &redacted, n
	} else if body.Body != nil && !body.AcknowledgeSecret && vault.ContainsSecret(*body.Body) {
		secretConflict(w)
		return
	}
	d, err := r.Artifacts.Update(req.Context(), accountOf(req), id, body.Update)
	if err != nil {
		fail(w, err)
		return
	}
	r.indexArtifact(req, d)
	writeJSON(w, http.StatusOK, artifactRes{Detail: d, MovedToVault: moved})
}

func (r *Router) handleDeleteArtifact(w http.ResponseWriter, req *http.Request) {
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	if err := r.Artifacts.Delete(req.Context(), accountOf(req), id); err != nil {
		fail(w, err)
		return
	}
	if r.Index != nil {
		if err := r.Index.Delete(req.Context(), accountOf(req), id); err != nil {
			log.Printf("artifact unindex %s: %v", id, err)
		}
	}
	r.audit(req, audit.ArtifactDelete, map[string]any{"artifact_id": id})
	w.WriteHeader(http.StatusNoContent)
}
