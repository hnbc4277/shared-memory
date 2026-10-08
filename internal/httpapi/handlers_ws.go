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

	"sharedmemory/internal/auth"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/session"
)

const wsIdleTimeout = 5 * time.Minute

// handleWS upgrades to a JSON WebSocket for live ingest/recall (design §接入面).
// Auth: Authorization Bearer, or ?access_token=, or first message {"type":"auth","token":"..."}.
func (r *Router) handleWS(w http.ResponseWriter, req *http.Request) {
	token := wsToken(req)
	var accountID uuid.UUID
	needAuthMsg := token == ""
	if token != "" {
		id, err := r.Auth.AuthenticateAPIKey(req.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		accountID = id
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
		accountID, err = r.wsAuthFirst(ctx, conn)
		if err != nil {
			_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": "unauthorized"})
			_ = conn.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
	}

	_ = wsjson.Write(ctx, conn, map[string]any{"type": "ready", "account_id": accountID})

	for {
		ctx, cancel := context.WithTimeout(req.Context(), wsIdleTimeout)
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
			r.wsRecall(req.Context(), conn, accountID, msg)
		case "ingest":
			r.wsIngest(req.Context(), conn, accountID, msg)
		default:
			_ = wsjson.Write(req.Context(), conn, map[string]any{"type": "error", "error": "unknown type; use ping|recall|ingest"})
		}
	}
}

func wsToken(req *http.Request) string {
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(req.URL.Query().Get("access_token"))
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

func (r *Router) wsAuthFirst(ctx context.Context, conn *websocket.Conn) (uuid.UUID, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var msg map[string]any
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		return uuid.Nil, err
	}
	if strings.ToLower(strAny(msg["type"])) != "auth" {
		return uuid.Nil, auth.ErrUnauthorized
	}
	token := strAny(msg["token"])
	if token == "" {
		token = strAny(msg["api_key"])
	}
	return r.Auth.AuthenticateAPIKey(ctx, token)
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
	res, err := r.Recall.Recall(ctx, accountID, reqBody)
	if err != nil {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": err.Error()})
		return
	}
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
	res, err := r.Sessions.Ingest(ctx, accountID, body)
	if err != nil {
		_ = wsjson.Write(ctx, conn, map[string]any{"type": "error", "error": err.Error()})
		return
	}
	_ = wsjson.Write(ctx, conn, map[string]any{"type": "ingest", "ok": true, "data": res})
}
