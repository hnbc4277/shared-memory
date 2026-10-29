package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"sharedmemory/internal/agents"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/session"
)

const wsIdleTimeout = 5 * time.Minute

// handleWS upgrades to a JSON WebSocket for live ingest/recall (design §接入面).
// Auth: Bearer API Key, web session (Cookie sm_session / wss_), ?access_token=, or first message {"type":"auth","token":"..."}.
func (r *Router) handleWS(w http.ResponseWriter, req *http.Request) {
	token := wsToken(req)
	var accountID uuid.UUID
	var apiKeyID *uuid.UUID
	needAuthMsg := token == ""
	if token != "" {
		id, keyID, ok := wsAuthenticate(req.Context(), r.Auth, token)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		accountID = id
		apiKeyID = keyID
	}

	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		OriginPatterns: r.wsOrigins(),
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := req.Context()
	if needAuthMsg {
		accountID, apiKeyID, err = r.wsAuthFirst(ctx, conn)
		if err != nil {
			_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "unauthorized"})
			_ = conn.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
	}

	_ = wsjson.Write(ctx, conn, map[string]any{"type": "ready", "account_id": accountID})

	wsCtx := req.Context()
	if apiKeyID != nil {
		wsCtx = withAPIKeyID(wsCtx, *apiKeyID)
	}

	for {
		ctx, cancel := context.WithTimeout(wsCtx, wsIdleTimeout)
		var msg map[string]any
		err := wsjson.Read(ctx, conn, &msg)
		cancel()
		if err != nil {
			return
		}
		typ, _ := msg["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))
		switch typ {
		case "ping":
			_ = wsjson.Write(req.Context(), conn, map[string]any{"type": "pong"})
		case "recall":
			r.wsRecall(wsCtx, conn, accountID, msg)
		case "ingest":
			r.wsIngest(wsCtx, conn, accountID, msg)
		default:
			_ = wsjson.Write(req.Context(), conn, map[string]any{"type": "error", "error": "unknown type; use ping|recall|ingest"})
		}
	}
}

func wsToken(req *http.Request) string {
	if t := bearerOrCookieToken(req); t != "" {
		return t
	}
	return strings.TrimSpace(req.URL.Query().Get("access_token"))
}

func wsAuthenticate(ctx context.Context, store *auth.Store, token string) (uuid.UUID, *uuid.UUID, bool) {
	authCtx, ok := authenticateRequest(ctx, store, token)
	if !ok {
		return uuid.Nil, nil, false
	}
	id, _ := AccountIDFromContext(authCtx)
	var keyID *uuid.UUID
	if kid, ok := APIKeyIDFromContext(authCtx); ok {
		keyID = &kid
	}
	return id, keyID, true
}

func (r *Router) wsOrigins() []string {
	if len(r.CORSOrigins) == 0 {
		return []string{"*"}
	}
	out := make([]string, 0, len(r.CORSOrigins))
	for _, o := range r.CORSOrigins {
		o = strings.TrimSpace(o)
		if o == "*" || o == "" {
			return []string{"*"}
		}
		out = append(out, o)
	}
	return out
}

func (r *Router) wsAuthFirst(ctx context.Context, conn *websocket.Conn) (uuid.UUID, *uuid.UUID, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var msg map[string]any
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		return uuid.Nil, nil, err
	}
	if strings.ToLower(strAny(msg["type"])) != "auth" {
		return uuid.Nil, nil, auth.ErrUnauthorized
	}
	token := strAny(msg["token"])
	if token == "" {
		token = strAny(msg["api_key"])
	}
	accountID, keyID, ok := wsAuthenticate(ctx, r.Auth, token)
	if !ok {
		return uuid.Nil, nil, auth.ErrUnauthorized
	}
	return accountID, keyID, nil
}

func (r *Router) wsApplyDedicatedConnection(ctx context.Context, accountID uuid.UUID, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return raw
	}
	if def := r.defaultConnectionID(ctx, accountID); def != nil {
		return def.String()
	}
	return ""
}

func strAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func (r *Router) wsRecall(ctx context.Context, conn *websocket.Conn, accountID uuid.UUID, msg map[string]any) {
	query := strAny(msg["query"])
	if query == "" {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "query required"})
		return
	}
	reqBody := recall.Request{Query: query, TopK: 8, Source: "ws"}
	if p := strAny(msg["project_id"]); p != "" {
		id, err := uuid.Parse(p)
		if err != nil {
			_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "project_id must be a uuid"})
			return
		}
		reqBody.ProjectID = &id
	}
	if n, ok := msg["top_k"].(float64); ok && int(n) > 0 {
		reqBody.TopK = int(n)
	}
	cid := r.wsApplyDedicatedConnection(ctx, accountID, strAny(msg["connection_id"]))
	if cid != "" {
		if _, err := agents.ParseConnectionID(cid); err != nil {
			_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "invalid connection_id"})
			return
		}
		if r.Agents != nil {
			id, _ := agents.ParseConnectionID(cid)
			if err := r.Agents.ValidateConnection(ctx, accountID, id); err != nil {
				_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "unknown connection_id"})
				return
			}
		}
		reqBody.ConnectionID = cid
	}
	res, err := r.Recall.Recall(ctx, accountID, reqBody)
	if err != nil {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": err.Error()})
		return
	}
	r.recordRecallAgentsCtx(ctx, accountID, reqBody)
	_ = wsjson.Write(ctx, conn, map[string]any{"type": "recall", "ok": true, "data": res})
}

func (r *Router) wsIngest(ctx context.Context, conn *websocket.Conn, accountID uuid.UUID, msg map[string]any) {
	raw, err := json.Marshal(msg)
	if err != nil {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "invalid message"})
		return
	}
	var body session.IngestMessageRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "invalid ingest payload"})
		return
	}
	body.ConnectionID = r.wsApplyDedicatedConnection(ctx, accountID, body.ConnectionID)
	if body.ConnectionID != "" {
		if _, err := agents.ParseConnectionID(body.ConnectionID); err != nil {
			_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "invalid connection_id"})
			return
		}
		if r.Agents != nil {
			id, _ := agents.ParseConnectionID(body.ConnectionID)
			if err := r.Agents.ValidateConnection(ctx, accountID, id); err != nil {
				_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "unknown connection_id"})
				return
			}
		}
	}
	res, err := r.Sessions.Ingest(ctx, accountID, body)
	if err != nil {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": err.Error()})
		return
	}
	r.recordIngestAgentsCtx(ctx, accountID, body)
	_ = wsjson.Write(ctx, conn, map[string]any{"type": "ingest", "ok": true, "data": res})
}
