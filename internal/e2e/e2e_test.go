//go:build e2e

// Package e2e drives the real HTTP API against the project's compose stack:
// docker compose up -d && go test -tags=e2e ./internal/e2e/ -v
//
// Tests use a dedicated Redis job stream so a stale compose "worker" container
// (old image without kb_parse) cannot steal queued jobs from this process.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"sharedmemory/internal/app"
	"sharedmemory/internal/config"
	"sharedmemory/internal/index"
	"sharedmemory/internal/objectstore"
)

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:       ":0",
		PostgresDSN:    "postgres://sm:sm@127.0.0.1:15432/sharedmemory?sslmode=disable",
		RedisAddr:      "127.0.0.1:16379",
		JobStream:      "sm:jobs:e2e",
		JobGroup:       "sm-workers-e2e",
		MinioEndpoint:  "127.0.0.1:19000",
		MinioAccessKey: "smminio",
		MinioSecretKey: "smminio_secret",
		MinioBucket:    "shared-memory",
		OpenSearchURL:  "http://127.0.0.1:19200",
		QdrantURL:      "http://127.0.0.1:16333",
		VaultMasterKey: "0123456789abcdef0123456789abcdef",
	}
}

type client struct {
	t    *testing.T
	base string
	key  string
}

func (c client) do(method, path string, body any, headers ...string) (int, []byte) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func (c client) uploadKB(title, filename, content string) (int, []byte) {
	c.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if title != "" {
		_ = w.WriteField("title", title)
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		c.t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		c.t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/v1/kb/documents", &buf)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func (c client) must(status int, method, path string, body any, out any, headers ...string) {
	c.t.Helper()
	got, raw := c.do(method, path, body, headers...)
	if got != status {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, got, status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode: %v: %s", method, path, err, raw)
		}
	}
}

func register(t *testing.T, base string) client {
	c := client{t: t, base: base}
	var res struct {
		AccountID string `json:"account_id"`
		APIKey    string `json:"api_key"`
	}
	c.must(201, "POST", "/v1/auth/register", map[string]string{
		"email": "e2e-" + uuid.NewString() + "@example.com", "password": "password123"}, &res)
	c.key = res.APIKey
	return c
}

type ingestRes struct {
	SessionID string   `json:"session_id"`
	MessageID string   `json:"message_id"`
	Warnings  []string `json:"warnings"`
	Prompts   []struct {
		Kind        string `json:"kind"`
		CandidateID string `json:"candidate_id"`
	} `json:"prompts"`
}

func msg(projectID, role, content string) map[string]any {
	return map[string]any{
		"project_id": projectID, "agent": "xiaolongxia", "agent_path": "xiaolongxia/researcher",
		"skills": []string{"deep-research"}, "plugins": []string{"browser"}, "tools": []string{"web_fetch"},
		"model": "deepseek-v3", "role": role, "content": content,
	}
}

func TestEndToEnd(t *testing.T) {
	ctx := context.Background()
	a, err := app.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("start app (is `docker compose up -d` running?): %v", err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	drain := func() {
		if _, err := a.Streams.Drain(ctx, "e2e", a.Worker.Handle); err != nil {
			t.Fatal(err)
		}
	}

	// 1-2. two independent accounts
	A := register(t, srv.URL)
	B := register(t, srv.URL)
	var me struct {
		AccountID string `json:"account_id"`
	}
	A.must(200, "GET", "/v1/me", nil, &me)
	accountA := uuid.MustParse(me.AccountID)

	var proj struct {
		ID string `json:"id"`
	}
	A.must(201, "POST", "/v1/projects", map[string]string{"name": "e2e"}, &proj)

	// 3. provenance is mandatory; a sub-agent message is accepted
	bad := msg(proj.ID, "user", "x")
	delete(bad, "skills")
	if code, _ := A.do("POST", "/v1/ingest", bad); code != 400 {
		t.Fatalf("missing skills must be rejected, got %d", code)
	}
	marker := "e2emark" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	var r1 ingestRes
	A.must(201, "POST", "/v1/ingest", msg(proj.ID, "user", "我们决定采用 OpenSearch 做全文检索 "+marker), &r1)
	drain()

	var sessions []struct {
		ID         string   `json:"id"`
		AgentPaths []string `json:"agent_paths"`
	}
	A.must(200, "GET", "/v1/sessions?agent_path=xiaolongxia", nil, &sessions)
	if len(sessions) != 1 || sessions[0].AgentPaths[0] != "xiaolongxia/researcher" {
		t.Fatalf("sessions = %+v", sessions)
	}
	var detail struct {
		Messages []struct {
			Skills  []string `json:"skills"`
			Plugins []string `json:"plugins"`
			Tools   []string `json:"tools"`
			Content *string  `json:"content"`
		} `json:"messages"`
	}
	A.must(200, "GET", "/v1/sessions/"+r1.SessionID, nil, &detail)
	if m := detail.Messages[0]; m.Skills[0] != "deep-research" || m.Plugins[0] != "browser" || m.Tools[0] != "web_fetch" || m.Content == nil {
		t.Fatalf("session detail = %+v", detail)
	}
	if code, _ := B.do("GET", "/v1/sessions/"+r1.SessionID, nil); code != 404 {
		t.Fatalf("B reading A's session: %d", code)
	}

	// 4. recall hits for A, nothing for B
	var rec struct {
		Hits []struct {
			Body string `json:"body"`
		} `json:"hits"`
		Context string `json:"context"`
	}
	A.must(200, "POST", "/v1/recall", map[string]any{"project_id": proj.ID, "query": marker + " OpenSearch"}, &rec)
	if len(rec.Hits) == 0 || !strings.Contains(rec.Context, marker) {
		t.Fatalf("A recall = %+v", rec)
	}
	_, rawB := B.do("POST", "/v1/recall", map[string]any{"query": marker + " OpenSearch"})
	if strings.Contains(string(rawB), marker) {
		t.Fatalf("B recall leaked A's data: %s", rawB)
	}
	if code, _ := B.do("POST", "/v1/recall", map[string]any{"project_id": proj.ID, "query": marker}); code != 404 {
		t.Fatalf("B recalling A's project: %d", code)
	}

	// 5. transcript off -> no new objects
	countObjects := func() int {
		keys, err := a.Objects.ListKeys(ctx, objectstore.AccountPrefix(accountA))
		if err != nil {
			t.Fatal(err)
		}
		return len(keys)
	}
	before := countObjects()
	A.must(200, "PUT", "/v1/policy", map[string]any{"store_transcript": false}, nil)
	A.must(201, "POST", "/v1/ingest", msg(proj.ID, "user", "这条不保存原文"), nil)
	if after := countObjects(); after != before {
		t.Fatalf("transcript off wrote objects: %d -> %d", before, after)
	}
	var pol map[string]any
	A.must(200, "GET", "/v1/policy", nil, &pol)
	if pol["store_transcript"] != false || pol["inject_enabled"] != true {
		t.Fatalf("partial policy update clobbered other toggles: %v", pol)
	}
	A.must(200, "PUT", "/v1/policy", map[string]any{"store_transcript": true}, nil)

	// 6. vault: consent, encrypted storage, password-gated reveal, never in recall
	secret := "Zx9!" + uuid.NewString()[:6]
	var r2 ingestRes
	A.must(201, "POST", "/v1/ingest", msg(proj.ID, "user", "GitLab https://gitlab.example.com 账号: alice 密码: "+secret), &r2)
	if len(r2.Prompts) != 1 || r2.Prompts[0].Kind != "vault" {
		t.Fatalf("vault prompt missing: %+v", r2)
	}
	drain()
	var item struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	A.must(201, "POST", "/v1/vault/candidates/"+r2.Prompts[0].CandidateID+"/accept", nil, &item)
	_, rawItems := A.do("GET", "/v1/vault/items", nil)
	if strings.Contains(string(rawItems), secret) || item.Username != "alice" {
		t.Fatalf("vault list must not expose secrets: %s", rawItems)
	}
	if code, _ := A.do("POST", "/v1/vault/items/"+item.ID+"/reveal", map[string]string{"password": "wrong"}); code != 401 {
		t.Fatalf("reveal with wrong password: %d", code)
	}
	var revealed struct {
		Secret string `json:"secret"`
	}
	A.must(200, "POST", "/v1/vault/items/"+item.ID+"/reveal", map[string]string{"password": "password123"}, &revealed)
	if revealed.Secret != secret {
		t.Fatalf("revealed %q", revealed.Secret)
	}
	if code, _ := B.do("POST", "/v1/vault/items/"+item.ID+"/reveal", map[string]string{"password": "password123"}); code == 200 {
		t.Fatal("B revealed A's secret")
	}
	_, rawRecall := A.do("POST", "/v1/recall", map[string]any{"project_id": proj.ID, "query": "GitLab 密码 alice"})
	_, rawSession := A.do("GET", "/v1/sessions/"+r2.SessionID, nil)
	for name, raw := range map[string][]byte{"recall": rawRecall, "session": rawSession} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("plaintext secret leaked through %s: %s", name, raw)
		}
	}
	A.must(204, "POST", "/v1/vault/items/"+item.ID+"/copied", nil, nil)
	if code, _ := B.do("POST", "/v1/vault/items/"+item.ID+"/copied", nil); code != 404 {
		t.Fatalf("B auditing a copy of A's secret: %d", code)
	}
	var auditLog []struct {
		Action string `json:"action"`
	}
	A.must(200, "GET", "/v1/audit", nil, &auditLog)
	reveals, copies := 0, 0
	for _, e := range auditLog {
		switch e.Action {
		case "vault.reveal":
			reveals++
		case "vault.copy":
			copies++
		}
	}
	if reveals != 2 || copies != 1 {
		t.Fatalf("expected 2 audited reveals and 1 copy, got %d and %d", reveals, copies)
	}

	// 7. artifact: offered for a long reply, kept only after consent
	var r3 ingestRes
	long := "# Redis Streams 学习笔记\n" + strings.Repeat("消费者组、XACK 与 XAUTOCLAIM 的用法。", 60)
	A.must(201, "POST", "/v1/ingest", msg(proj.ID, "assistant", long), &r3)
	var arts []any
	A.must(200, "GET", "/v1/artifacts", nil, &arts)
	if len(arts) != 0 || len(r3.Prompts) != 1 || r3.Prompts[0].Kind != "artifact" {
		t.Fatalf("artifact must wait for consent: %v %+v", arts, r3.Prompts)
	}
	var acc struct {
		Artifact struct {
			ID string `json:"id"`
		} `json:"artifact"`
	}
	A.must(201, "POST", "/v1/artifacts/candidates/"+r3.Prompts[0].CandidateID+"/accept", map[string]any{"tags": []string{"redis"}}, &acc)
	var got struct {
		Body string `json:"body"`
	}
	A.must(200, "GET", "/v1/artifacts/"+acc.Artifact.ID, nil, &got)
	if got.Body != long {
		t.Fatal("artifact body mismatch")
	}
	if code, _ := B.do("GET", "/v1/artifacts/"+acc.Artifact.ID, nil); code != 404 {
		t.Fatalf("B reading A's artifact: %d", code)
	}
	if code, _ := A.do("POST", "/v1/artifacts", map[string]any{"title": "x", "body": "token: abc123456"}); code != 409 {
		t.Fatalf("artifact with secret must need acknowledgement, got %d", code)
	}
	var vaultBefore []any
	A.must(200, "GET", "/v1/vault/items", nil, &vaultBefore)
	var moved struct {
		Body         string `json:"body"`
		MovedToVault int    `json:"moved_to_vault"`
	}
	A.must(201, "POST", "/v1/artifacts", map[string]any{"title": "部署笔记",
		"body": "登录 https://ci.example.com 账号: bot 密码: Ci#77abc", "move_secrets_to_vault": true}, &moved)
	var vaultAfter []any
	A.must(200, "GET", "/v1/vault/items", nil, &vaultAfter)
	if moved.MovedToVault != 1 || strings.Contains(moved.Body, "Ci#77abc") || len(vaultAfter) != len(vaultBefore)+1 {
		t.Fatalf("secret must move to the vault and leave the artifact redacted: %+v vault %d->%d",
			moved, len(vaultBefore), len(vaultAfter))
	}

	// 8. delete A -> everything gone, B untouched
	if code, _ := A.do("DELETE", "/v1/account", nil); code != 428 {
		t.Fatalf("delete without confirmation: %d", code)
	}
	A.must(204, "DELETE", "/v1/account", nil, nil, "X-Confirm", "delete")
	if code, _ := A.do("GET", "/v1/sessions", nil); code != 401 {
		t.Fatalf("deleted account's key still works: %d", code)
	}
	if n := countObjects(); n != 0 {
		t.Fatalf("%d objects left for deleted account", n)
	}
	hits, err := a.Index.Search(ctx, index.Query{AccountID: accountA, Text: marker, Limit: 10})
	if err != nil || len(hits) != 0 {
		t.Fatalf("index still has deleted account docs: %v %v", hits, err)
	}
	var n int
	if err := a.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE account_id = $1`, accountA).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sessions left: %d %v", n, err)
	}
	B.must(200, "GET", "/v1/sessions", nil, nil)
	fmt.Println("e2e: all 8 acceptance steps passed")
}

func TestKnowledgeBaseUploadRecall(t *testing.T) {
	ctx := context.Background()
	a, err := app.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("start app (is `docker compose up -d` running?): %v", err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	drain := func() {
		if _, err := a.Streams.Drain(ctx, "e2e-kb", a.Worker.Handle); err != nil {
			t.Fatal(err)
		}
	}

	A := register(t, srv.URL)
	B := register(t, srv.URL)
	marker := "kbe2e" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	body := "# 知识库验收\n\n项目规范要求所有 API 必须带 account_id 隔离。" + marker + "\n"

	code, raw := A.uploadKB("E2E KB", "spec.md", body)
	if code != 201 {
		t.Fatalf("upload kb: status %d: %s", code, raw)
	}
	var created struct {
		Document struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"document"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode upload: %v: %s", err, raw)
	}
	docID := created.Document.ID
	if docID == "" {
		t.Fatalf("missing document id: %s", raw)
	}

	drain()

	var doc struct {
		Document struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"document"`
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		A.must(200, "GET", "/v1/kb/documents/"+docID, nil, &doc)
		switch doc.Document.Status {
		case "ready":
			goto ready
		case "failed":
			t.Fatalf("kb parse failed: %s", doc.Document.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("kb document still %q after timeout", doc.Document.Status)
		}
		time.Sleep(200 * time.Millisecond)
		drain()
	}
ready:

	var rec struct {
		KBHits []struct {
			Body string `json:"body"`
		} `json:"kb_hits"`
		Context string `json:"context"`
	}
	A.must(200, "POST", "/v1/recall", map[string]any{"query": marker + " account_id"}, &rec)
	if len(rec.KBHits) == 0 {
		t.Fatalf("expected kb_hits, recall = %+v", rec)
	}
	found := strings.Contains(rec.Context, marker)
	for _, h := range rec.KBHits {
		if strings.Contains(h.Body, marker) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("recall missing kb marker: hits=%+v context=%q", rec.KBHits, rec.Context)
	}

	if code, _ := B.do("GET", "/v1/kb/documents/"+docID, nil); code != 404 {
		t.Fatalf("B reading A's kb document: %d", code)
	}
	_, rawB := B.do("POST", "/v1/recall", map[string]any{"query": marker + " account_id"})
	if strings.Contains(string(rawB), marker) {
		t.Fatalf("B recall leaked A's kb: %s", rawB)
	}
}
