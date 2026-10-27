package mcp

import (
	"context"
	"net/http"
)

// PostConnectionHeartbeat POSTs /v1/agents/connections/heartbeat with client "mcp".
func PostConnectionHeartbeat(c *Client, ctx context.Context, connectionID string) error {
	body := map[string]string{"client": "mcp"}
	if connectionID != "" {
		body["connection_id"] = connectionID
	}
	_, err := c.CallRaw(ctx, http.MethodPost, "/v1/agents/connections/heartbeat", body)
	return err
}
