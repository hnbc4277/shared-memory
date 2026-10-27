package httpapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"sharedmemory/internal/agents"
	"sharedmemory/internal/artifact"
	"sharedmemory/internal/cred"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/session"
)

func (r *Router) handleListAgents(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	list, err := r.Agents.ListGrouped(req.Context(), accountOf(req))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": list})
}

func (r *Router) handleCreateAgentConnection(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	var body struct {
		Agent             string `json:"agent"`
		Label             string `json:"label"`
		IssueDedicatedKey bool   `json:"issue_dedicated_key"`
		ClaimHistory      bool   `json:"claim_history"`
	}
	if !decode(w, req, &body) {
		return
	}
	accountID := accountOf(req)
	res, err := r.Agents.CreateRegistered(req.Context(), accountID, agents.CreateInput{
		Agent: body.Agent, Label: body.Label, IssueDedicatedKey: body.IssueDedicatedKey,
	})
	if err != nil {
		fail(w, err)
		return
	}
	if body.ClaimHistory {
		if _, err := r.Agents.ClaimHistory(req.Context(), accountID, res.Connection.ID, nil); err != nil {
			fail(w, err)
			return
		}
		r.asyncReindexClaimedConnection(accountID, res.Connection.ID)
	}
	if r.attachCursorSetupScript(req.Context(), accountID, body.Agent, res.Connection.ID, res.APIKey) {
		res.Connection.HasSetupScript = true
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"connection":       res.Connection,
		"connection_id":    res.Connection.ID,
		"api_key":          res.APIKey,
		"has_setup_script": res.Connection.HasSetupScript,
	})
}

func (r *Router) handlePatchAgentConnection(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if !decode(w, req, &body) {
		return
	}
	c, err := r.Agents.PatchLabel(req.Context(), accountOf(req), id, body.Label)
	if errors.Is(err, agents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": c})
}

func (r *Router) handleUpgradeAgentConnection(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var body struct {
		Label             string `json:"label"`
		IssueDedicatedKey bool   `json:"issue_dedicated_key"`
		ClaimHistory      bool   `json:"claim_history"`
	}
	if req.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(req.Body, 8<<20))
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
				return
			}
		}
	}
	res, err := r.Agents.UpgradeAuto(req.Context(), accountOf(req), id, body.Label, body.IssueDedicatedKey, body.ClaimHistory)
	if errors.Is(err, agents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, agents.ErrInvalid) {
		writeError(w, http.StatusBadRequest, "only auto connections can be upgraded")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if body.ClaimHistory {
		r.asyncReindexClaimedConnection(accountOf(req), res.Connection.ID)
	}
	if r.attachCursorSetupScript(req.Context(), accountOf(req), res.Connection.Agent, res.Connection.ID, res.APIKey) {
		res.Connection.HasSetupScript = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connection":       res.Connection,
		"connection_id":    res.Connection.ID,
		"api_key":          res.APIKey,
		"has_setup_script": res.Connection.HasSetupScript,
		"hint":             "Set SHARED_MEMORY_CONNECTION_ID or pass connection_id on ingest/recall",
	})
}

func (r *Router) attachCursorSetupScript(ctx context.Context, accountID uuid.UUID, agent string, connectionID uuid.UUID, apiKey string) bool {
	if r.Agents == nil || !strings.EqualFold(strings.TrimSpace(agent), "cursor") || strings.TrimSpace(apiKey) == "" {
		return false
	}
	// Binaries are served by the API at /downloads/*; bake public URL for remote clients.
	apiURL := strings.TrimRight(strings.TrimSpace(r.PublicAPIURL), "/")
	if apiURL == "" {
		apiURL = cred.DefaultAPIURL
	}
	script := agents.RenderCursorSetupScript(apiURL, apiKey, connectionID.String(), apiURL)
	if err := r.Agents.SaveSetupScript(ctx, accountID, connectionID, script); err != nil {
		log.Printf("save cursor setup script %s: %v", connectionID, err)
		return false
	}
	return true
}

func (r *Router) handleDownloadConnectionSetupScript(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	script, label, err := r.Agents.GetSetupScript(req.Context(), accountOf(req), id)
	if errors.Is(err, agents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "setup script not available; recreate the Cursor connection with a dedicated API key")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if strings.Contains(script, "__SM_API_KEY__") || strings.Contains(script, "__SM_CONNECTION_ID__") {
		writeError(w, http.StatusConflict, "setup script is incomplete; recreate the Cursor connection with a dedicated API key")
		return
	}
	// Re-render with current template so old packages get the sentinel fix + single-quoted bakes.
	if apiURL, apiKey, connID, dlBase, ok := agents.ParseCursorSetupScriptValues(script); ok {
		if pub := strings.TrimRight(strings.TrimSpace(r.PublicAPIURL), "/"); pub != "" {
			if apiURL == "" || strings.Contains(apiURL, "127.0.0.1") || strings.Contains(apiURL, "localhost") {
				apiURL = pub
			}
			if dlBase == "" || strings.Contains(dlBase, "127.0.0.1") || strings.Contains(dlBase, "localhost") {
				dlBase = pub
			}
		}
		script = agents.RenderCursorSetupScript(apiURL, apiKey, connID, dlBase)
	}
	if pub := strings.TrimRight(strings.TrimSpace(r.PublicAPIURL), "/"); pub != "" {
		script = rewriteSetupScriptLocalURLs(script, pub)
	}
	base := agents.SetupScriptBaseName(id.String())
	ps1Name := base + ".ps1"
	cmdName := base + ".cmd"
	cmdBody := agents.RenderCursorSetupLauncher()
	displayLabel := strings.TrimSpace(label)
	if displayLabel == "" {
		displayLabel = id.String()
	}
	notice := fmt.Sprintf(
		"This package is for connection [%s] only.\r\n"+
			"connection_id and the dedicated API key are already inside the .ps1 — no need to type them.\r\n"+
			"Do NOT forward, upload, or commit this zip.\r\n"+
			"Unzip, then double-click %s to install.\r\n"+
			"\r\n"+
			"本包仅适用于连接「%s」。\r\n"+
			"脚本内已含 connection_id 与专用 API 密钥，执行时无需再填写。\r\n"+
			"请勿转发或上传。解压后双击 %s 即可。\r\n",
		displayLabel, cmdName, displayLabel, cmdName,
	)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, base))
	zw := zip.NewWriter(w)
	defer zw.Close()
	// UTF-8 BOM so Windows PowerShell 5.1 and Notepad read Chinese correctly.
	noticeBytes := append([]byte{0xEF, 0xBB, 0xBF}, []byte(notice)...)
	if err := writeZipFile(zw, "README.txt", noticeBytes); err != nil {
		log.Printf("setup zip notice: %v", err)
		return
	}
	if err := writeZipFile(zw, ps1Name, agents.PS1FileBytes(script)); err != nil {
		log.Printf("setup zip ps1: %v", err)
		return
	}
	if err := writeZipFile(zw, cmdName, []byte(cmdBody)); err != nil {
		log.Printf("setup zip cmd: %v", err)
		return
	}
}

func rewriteSetupScriptLocalURLs(script, public string) string {
	for _, old := range []string{
		"http://127.0.0.1:18080",
		"http://localhost:18080",
		"http://127.0.0.1:15173",
		"http://localhost:15173",
	} {
		script = strings.ReplaceAll(script, old, public)
	}
	return script
}

func writeZipFile(zw *zip.Writer, name string, body []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func (r *Router) asyncReindexClaimedConnection(accountID, connectionID uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if r.Memories != nil {
			cnt, err := r.Memories.ReindexBySourceConnection(ctx, accountID, connectionID)
			if err != nil {
				log.Printf("claim reindex memories connection %s: %v", connectionID, err)
			} else if cnt > 0 {
				log.Printf("claim reindexed %d memories for connection %s", cnt, connectionID)
			}
		}
		if r.Artifacts != nil {
			cnt, err := artifact.ReindexBySessionConnection(ctx, r.Pool, r.Index, r.Artifacts, accountID, connectionID)
			if err != nil {
				log.Printf("claim reindex artifacts connection %s: %v", connectionID, err)
			} else if cnt > 0 {
				log.Printf("claim reindexed %d artifacts for connection %s", cnt, connectionID)
			}
		}
	}()
}

func (r *Router) handleClaimAgentHistory(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	var body struct {
		Before *time.Time `json:"before"`
	}
	if req.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(req.Body, 8<<20))
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
				return
			}
		}
	}
	n, err := r.Agents.ClaimHistory(req.Context(), accountOf(req), id, body.Before)
	if errors.Is(err, agents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, agents.ErrInvalid) {
		writeError(w, http.StatusBadRequest, "only registered connections can claim history")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if n > 0 {
		r.asyncReindexClaimedConnection(accountOf(req), id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions_claimed": n})
}

func (r *Router) handleAgentsStream(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	accountID := accountOf(req)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	writeEvent := func() bool {
		groups, err := r.Agents.ListGrouped(req.Context(), accountID)
		if err != nil {
			_, _ = fmt.Fprintf(w, "event: error\ndata: %q\n\n", err.Error())
			flusher.Flush()
			return true
		}
		raw, _ := json.Marshal(map[string]any{"groups": groups})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !writeEvent() {
		return
	}
	for {
		select {
		case <-req.Context().Done():
			return
		case <-ticker.C:
			if !writeEvent() {
				return
			}
		}
	}
}

func (r *Router) handleAgentsHeartbeat(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	var body struct {
		ConnectionID string `json:"connection_id"`
		Client       string `json:"client"`
		Version      string `json:"version"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)
	accountID := accountOf(req)
	raw := strings.TrimSpace(body.ConnectionID)
	if q := strings.TrimSpace(req.URL.Query().Get("connection_id")); raw == "" && q != "" {
		raw = q
	}
	if raw == "" {
		if def := r.defaultConnectionID(req.Context(), accountID); def != nil {
			raw = def.String()
		}
	}
	if raw == "" {
		writeError(w, http.StatusBadRequest, "connection_id required")
		return
	}
	id, err := agents.ParseConnectionID(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection_id")
		return
	}
	if keyID, ok := APIKeyIDFromContext(req.Context()); ok && r.Agents != nil {
		if bound, err := r.Agents.ConnectionIDForAPIKey(req.Context(), accountID, keyID); err == nil && bound != nil && *bound != id {
			writeError(w, http.StatusForbidden, "connection_id does not match API key")
			return
		}
	}
	if err := r.Agents.TouchHeartbeat(req.Context(), accountID, id); err != nil {
		if errors.Is(err, agents.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleDeleteAgentConnection(w http.ResponseWriter, req *http.Request) {
	if r.Agents == nil {
		writeError(w, http.StatusServiceUnavailable, "agents unavailable")
		return
	}
	id, ok := pathID(w, req)
	if !ok {
		return
	}
	revokeKey := strings.EqualFold(req.URL.Query().Get("revoke_api_key"), "true") ||
		strings.EqualFold(req.URL.Query().Get("revoke_api_key"), "1")
	if err := r.Agents.RevokeMaybeKey(req.Context(), accountOf(req), id, revokeKey); err != nil {
		if errors.Is(err, agents.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) defaultConnectionID(ctx context.Context, accountID uuid.UUID) *uuid.UUID {
	if r.Agents == nil {
		return nil
	}
	keyID, ok := APIKeyIDFromContext(ctx)
	if !ok {
		return nil
	}
	cid, err := r.Agents.ConnectionIDForAPIKey(ctx, accountID, keyID)
	if err != nil || cid == nil {
		return nil
	}
	return cid
}

// resolveConnectionID applies body connection_id or dedicated-key default, then validates.
func (r *Router) resolveConnectionID(w http.ResponseWriter, req *http.Request, accountID uuid.UUID, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if def := r.defaultConnectionID(req.Context(), accountID); def != nil {
			raw = def.String()
		} else {
			return "", true
		}
	}
	cid, ok := r.validateConnectionID(w, req, accountID, raw)
	if !ok {
		return "", false
	}
	return cid.String(), true
}

func (r *Router) validateConnectionID(w http.ResponseWriter, req *http.Request, accountID uuid.UUID, raw string) (*uuid.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	id, err := agents.ParseConnectionID(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection_id")
		return nil, false
	}
	if r.Agents == nil {
		writeError(w, http.StatusBadRequest, "invalid connection_id")
		return nil, false
	}
	if err := r.Agents.ValidateConnection(req.Context(), accountID, id); err != nil {
		if errors.Is(err, agents.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "unknown connection_id")
			return nil, false
		}
		fail(w, err)
		return nil, false
	}
	return &id, true
}

func (r *Router) recordIngestAgents(req *http.Request, accountID uuid.UUID, body session.IngestMessageRequest) {
	r.recordIngestAgentsCtx(req.Context(), accountID, body)
}

func (r *Router) recordIngestAgentsCtx(ctx context.Context, accountID uuid.UUID, body session.IngestMessageRequest) {
	if r.Agents == nil {
		return
	}
	var connID *uuid.UUID
	if body.ConnectionID != "" {
		id, err := agents.ParseConnectionID(body.ConnectionID)
		if err == nil {
			connID = &id
		}
	}
	_ = r.Agents.RecordIngest(ctx, accountID, connID, agents.Activity{
		Agent: body.Agent, AgentPath: body.AgentPath, Model: body.Model,
		ExternalSessionID: body.ExternalSessionID,
	})
}

func (r *Router) recordRecallAgents(req *http.Request, accountID uuid.UUID, body recall.Request) {
	r.recordRecallAgentsCtx(req.Context(), accountID, body)
}

func (r *Router) recordRecallAgentsCtx(ctx context.Context, accountID uuid.UUID, body recall.Request) {
	if r.Agents == nil {
		return
	}
	var connID *uuid.UUID
	if body.ConnectionID != "" {
		id, err := agents.ParseConnectionID(body.ConnectionID)
		if err == nil {
			connID = &id
		}
	}
	_ = r.Agents.RecordRecall(ctx, accountID, connID, agents.Activity{
		Agent: body.Agent, AgentPath: body.AgentPath,
		ExternalSessionID: body.ExternalSessionID,
	})
}
