package smctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"sharedmemory/internal/cred"
)

// Login runs the device-code flow and saves credentials.
func Login(ctx context.Context, apiURL, clientName, agent string, out io.Writer) error {
	apiURL = strings.TrimRight(firstNonEmpty(apiURL, os.Getenv(cred.EnvAPIURL), cred.DefaultAPIURL), "/")
	if clientName == "" {
		clientName = "smctl"
	}
	client := &http.Client{Timeout: 15 * time.Second}

	codeBody, _ := json.Marshal(map[string]any{"client_name": clientName, "agent": agent})
	res, err := client.Post(apiURL+"/v1/device/code", "application/json", bytes.NewReader(codeBody))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("device/code: %s %s", res.Status, b)
	}
	var code struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := json.NewDecoder(res.Body).Decode(&code); err != nil {
		return err
	}
	uri := code.VerificationURIComplete
	if uri == "" {
		uri = code.VerificationURI
	}
	fmt.Fprintf(out, "User code: %s\n", code.UserCode)
	fmt.Fprintf(out, "Open: %s\n", uri)
	fmt.Fprintf(out, "Waiting for approval...\n")

	interval := time.Duration(code.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
	if code.ExpiresIn <= 0 {
		deadline = time.Now().Add(10 * time.Minute)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("device code expired")
		}
		tok, pending, err := pollToken(ctx, client, apiURL, code.DeviceCode)
		if err != nil {
			return err
		}
		if pending {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
			continue
		}
		c := cred.Credentials{
			APIKey:   tok.APIKey,
			APIURL:   apiURL,
			DeviceID: tok.DeviceID,
		}
		if err := cred.Save(c); err != nil {
			return err
		}
		fmt.Fprintf(out, "Logged in. credentials saved to %s\n", mustCredPath())
		return nil
	}
}

type tokenResp struct {
	APIKey   string `json:"api_key"`
	DeviceID string `json:"device_id"`
}

func pollToken(ctx context.Context, client *http.Client, apiURL, deviceCode string) (tokenResp, bool, error) {
	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/v1/device/token", bytes.NewReader(body))
	if err != nil {
		return tokenResp{}, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return tokenResp{}, false, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusBadRequest {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		switch e.Error {
		case "authorization_pending":
			return tokenResp{}, true, nil
		case "expired_token":
			return tokenResp{}, false, fmt.Errorf("expired_token")
		default:
			return tokenResp{}, false, fmt.Errorf("device/token: %s", raw)
		}
	}
	if res.StatusCode >= 300 {
		return tokenResp{}, false, fmt.Errorf("device/token: %s %s", res.Status, raw)
	}
	var tok tokenResp
	if err := json.Unmarshal(raw, &tok); err != nil {
		return tokenResp{}, false, err
	}
	if tok.APIKey == "" {
		return tokenResp{}, false, fmt.Errorf("empty api_key")
	}
	return tok, false, nil
}

func mustCredPath() string {
	p, err := cred.Path()
	if err != nil {
		return "~/.shared-memory/credentials.json"
	}
	return p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Logout removes local credentials. If revoke is true and a device_id exists, DELETE /v1/devices/{id}.
func Logout(ctx context.Context, revoke bool) error {
	c, err := cred.Load()
	if err != nil && !cred.IsNotFound(err) {
		return err
	}
	if revoke && err == nil && c.DeviceID != "" && c.APIKey != "" {
		url := strings.TrimRight(c.APIURL, "/") + "/v1/devices/" + c.DeviceID
		req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
	}
	return cred.Delete()
}

// Status reports credentials, daemon health, and spool depth.
func Status(ctx context.Context, out io.Writer) error {
	c, err := cred.Resolve()
	if err != nil {
		if cred.IsNotFound(err) {
			fmt.Fprintln(out, "credentials: missing (run smctl login)")
		} else {
			fmt.Fprintf(out, "credentials: error: %v\n", err)
		}
	} else {
		fmt.Fprintf(out, "credentials: ok (api_url=%s device_id=%s)\n", c.APIURL, c.DeviceID)
	}

	daemonURL := firstNonEmpty(os.Getenv("SHARED_MEMORY_DAEMON_URL"), "http://127.0.0.1:18765")
	client := &http.Client{Timeout: 500 * time.Millisecond}
	res, err := client.Get(strings.TrimRight(daemonURL, "/") + "/healthz")
	if err != nil {
		fmt.Fprintf(out, "daemon: down (%s)\n", daemonURL)
	} else {
		defer res.Body.Close()
		var h struct {
			OK           bool `json:"ok"`
			SpoolPending int  `json:"spool_pending"`
		}
		_ = json.NewDecoder(res.Body).Decode(&h)
		fmt.Fprintf(out, "daemon: up spool_pending=%d\n", h.SpoolPending)
	}

	spoolDir, err := cred.SpoolDir()
	if err == nil {
		n := countJSONL(spoolDir)
		fmt.Fprintf(out, "spool_pending (local dir): %d\n", n)
	}

	if c, err := cred.Resolve(); err == nil && c.APIKey != "" {
		printProvenanceStats(ctx, out, c.APIURL, c.APIKey)
		printRecentReceipts(ctx, out, c.APIURL, c.APIKey)
	}
	return nil
}

func printRecentReceipts(ctx context.Context, out io.Writer, apiURL, apiKey string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiURL, "/")+"/v1/recall/receipts?limit=5", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintf(out, "recall_receipts: unreachable\n")
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		fmt.Fprintf(out, "recall_receipts: http %s\n", res.Status)
		return
	}
	var list []struct {
		QueriedAt time.Time `json:"queried_at"`
		Agent     string    `json:"agent"`
		HitCount  int       `json:"hit_count"`
		Degraded  bool      `json:"degraded"`
		Source    string    `json:"source"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		return
	}
	if len(list) == 0 {
		fmt.Fprintln(out, "recall_receipts: none recent")
		return
	}
	fmt.Fprintln(out, "recall_receipts (recent):")
	for _, r := range list {
		deg := "ok"
		if r.Degraded {
			deg = "degraded"
		}
		fmt.Fprintf(out, "  %s agent=%s hits=%d %s source=%s\n",
			r.QueriedAt.UTC().Format(time.RFC3339), r.Agent, r.HitCount, deg, r.Source)
	}
}

func printProvenanceStats(ctx context.Context, out io.Writer, apiURL, apiKey string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiURL, "/")+"/v1/provenance/stats", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintf(out, "provenance: unreachable\n")
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		fmt.Fprintf(out, "provenance: http %s\n", res.Status)
		return
	}
	var body struct {
		ByAgent []struct {
			Agent           string  `json:"agent"`
			Messages        int     `json:"messages"`
			UnknownModelPct float64 `json:"unknown_model_pct"`
			EmptySkillsPct  float64 `json:"empty_skills_pct"`
		} `json:"by_agent"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return
	}
	if len(body.ByAgent) == 0 {
		fmt.Fprintln(out, "provenance: no messages in last 7 days")
		return
	}
	fmt.Fprintln(out, "provenance (7d):")
	for _, a := range body.ByAgent {
		fmt.Fprintf(out, "  %s: msgs=%d unknown_model=%.0f%% empty_skills=%.0f%%\n",
			a.Agent, a.Messages, a.UnknownModelPct*100, a.EmptySkillsPct*100)
	}
}

func countJSONL(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n
}
