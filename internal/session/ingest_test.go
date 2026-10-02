//go:build integration

package session_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/session"
	"sharedmemory/internal/testutil"
	"sharedmemory/internal/vault"
)

type fixture struct {
	pool    *pgxpool.Pool
	objects *objectstore.Memory
	queue   *queue.Recorder
	svc     *session.Service
	vault   *vault.Store
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.TestPool(t)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	cipher, err := vault.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	vs := vault.NewStore(pool, cipher)
	return fixture{pool: pool, objects: objects, queue: rec, vault: vs, svc: &session.Service{
		Pool: pool, Objects: objects, Queue: rec, Policies: policy.NewStore(pool), Projects: project.NewStore(pool),
		Vault: vs, Artifacts: artifact.NewStore(pool, objects),
	}}
}

func baseReq(content string) session.IngestMessageRequest {
	return session.IngestMessageRequest{
		Agent: "xiaolongxia", AgentPath: "xiaolongxia/researcher",
		Skills: testutil.Strs("web-search"), Plugins: testutil.Strs("browser"), Tools: testutil.Strs("fetch", "grep"),
		Model: "deepseek-v3", Role: "user", Content: content,
	}
}

func TestIngest_StoresProvenanceAndTranscript(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	acct := testutil.NewAccount(t, f.pool)

	res, err := f.svc.Ingest(ctx, acct, baseReq("帮我调研一下 Qdrant 的过滤语法"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Seq != 1 {
		t.Fatalf("seq = %d, want 1", res.Seq)
	}
	req2 := baseReq("这是调研结果")
	req2.SessionID = res.SessionID.String()
	req2.Role = "assistant"
	req2.AgentPath = ""
	req2.Model = ""
	res2, err := f.svc.Ingest(ctx, acct, req2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Seq != 2 || res2.SessionID != res.SessionID {
		t.Fatalf("second message: %+v", res2)
	}

	d, err := f.svc.Get(ctx, acct, res.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Messages) != 2 {
		t.Fatalf("messages = %d", len(d.Messages))
	}
	m := d.Messages[0]
	if m.AgentPath != "xiaolongxia/researcher" || m.Agent != "xiaolongxia" || m.Model != "deepseek-v3" {
		t.Fatalf("provenance lost: %+v", m)
	}
	if strings.Join(m.Skills, ",") != "web-search" || strings.Join(m.Plugins, ",") != "browser" || strings.Join(m.Tools, ",") != "fetch,grep" {
		t.Fatalf("skills/plugins/tools lost: %+v", m)
	}
	if !m.ContentStored || m.Content == nil || *m.Content != "帮我调研一下 Qdrant 的过滤语法" {
		t.Fatalf("content not stored: %+v", m)
	}
	m2 := d.Messages[1]
	if m2.AgentPath != "xiaolongxia" || m2.Model != session.UnknownModel {
		t.Fatalf("normalization failed: %+v", m2)
	}
	if len(d.Session.AgentPaths) != 2 {
		t.Fatalf("agent paths = %v", d.Session.AgentPaths)
	}
	if len(f.queue.Jobs) != 2 || f.queue.Jobs[0].AgentPath != "xiaolongxia/researcher" {
		t.Fatalf("jobs = %+v", f.queue.Jobs)
	}
}

func TestIngest_TranscriptOffStoresNoObject(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	acct := testutil.NewAccount(t, f.pool)
	pol := policy.Default()
	pol.StoreTranscript = false
	if err := policy.NewStore(f.pool).Update(ctx, acct, pol); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Ingest(ctx, acct, baseReq("不要保存原文"))
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := f.objects.ListKeys(ctx, objectstore.AccountPrefix(acct))
	if len(keys) != 0 {
		t.Fatalf("transcript off must not write objects, got %v", keys)
	}
	d, err := f.svc.Get(ctx, acct, res.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Messages[0].ContentStored || d.Messages[0].Content != nil {
		t.Fatalf("content must be absent: %+v", d.Messages[0])
	}
	if d.Messages[0].AgentPath != "xiaolongxia/researcher" {
		t.Fatal("provenance must still be recorded with transcript off")
	}
	if len(f.queue.Jobs) != 1 || f.queue.Jobs[0].Content == "" {
		t.Fatal("semantic job must still carry content transiently")
	}
}

func TestIngest_RedactsSecretsAndOffersVault(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	acct := testutil.NewAccount(t, f.pool)
	res, err := f.svc.Ingest(ctx, acct, baseReq("登录 https://git.example.com 账号: alice 密码: S3cret!x"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Prompts) != 1 || res.Prompts[0].Kind != "vault" {
		t.Fatalf("expected one vault prompt, got %+v", res.Prompts)
	}
	keys, _ := f.objects.ListKeys(ctx, objectstore.AccountPrefix(acct))
	for _, k := range keys {
		rc, _ := f.objects.Get(ctx, k)
		buf := new(strings.Builder)
		_, _ = io.Copy(buf, rc)
		if strings.Contains(buf.String(), "S3cret!x") {
			t.Fatal("secret leaked into transcript")
		}
	}
	if strings.Contains(f.queue.Jobs[0].Content, "S3cret!x") {
		t.Fatal("secret leaked into queue")
	}
	cid := uuid.MustParse(res.Prompts[0].CandidateID)
	it, err := f.vault.Accept(ctx, acct, cid, vault.AcceptOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if it.Username != "alice" || it.SiteURL != "https://git.example.com" {
		t.Fatalf("vault item: %+v", it)
	}
	secret, err := f.vault.Reveal(ctx, acct, it.ID)
	if err != nil || secret != "S3cret!x" {
		t.Fatalf("reveal = %q, %v", secret, err)
	}
}

func TestIngest_QueueFailureMarksDegraded(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.queue.Err = errors.New("redis down")
	acct := testutil.NewAccount(t, f.pool)
	res, err := f.svc.Ingest(ctx, acct, baseReq("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Warnings, session.WarnQueueUnavailable) {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	d, _ := f.svc.Get(ctx, acct, res.SessionID)
	if !d.Session.Degraded {
		t.Fatal("session must be marked degraded")
	}
}

type failingPut struct{ *objectstore.Memory }

func (failingPut) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("minio down")
}

func TestIngest_TranscriptFailureMarksDegradedAndStoresNoPointer(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.svc.Objects = failingPut{f.objects}
	acct := testutil.NewAccount(t, f.pool)
	res, err := f.svc.Ingest(ctx, acct, baseReq("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Warnings, session.WarnTranscriptFailed) {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	d, _ := f.svc.Get(ctx, acct, res.SessionID)
	if !d.Session.Degraded {
		t.Fatal("a lost transcript must mark the session degraded")
	}
	if len(d.Messages) != 1 || d.Messages[0].ContentStored {
		t.Fatalf("message must not claim a stored transcript: %+v", d.Messages)
	}
}

func TestIngest_CannotWriteIntoAnotherAccountsSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a := testutil.NewAccount(t, f.pool)
	b := testutil.NewAccount(t, f.pool)
	res, err := f.svc.Ingest(ctx, a, baseReq("A 的消息"))
	if err != nil {
		t.Fatal(err)
	}
	req := baseReq("B 试图写入")
	req.SessionID = res.SessionID.String()
	if _, err := f.svc.Ingest(ctx, b, req); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.Get(ctx, b, res.SessionID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("B must not read A's session, err = %v", err)
	}
	list, _ := f.svc.List(ctx, b, session.ListFilter{})
	if len(list) != 0 {
		t.Fatalf("B sees %d sessions", len(list))
	}
}

func TestList_FiltersByAgentPathIncludingSubAgents(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	acct := testutil.NewAccount(t, f.pool)
	if _, err := f.svc.Ingest(ctx, acct, baseReq("子智能体的会话")); err != nil {
		t.Fatal(err)
	}
	cursor := baseReq("cursor 里的会话")
	cursor.Agent, cursor.AgentPath = "cursor", ""
	if _, err := f.svc.Ingest(ctx, acct, cursor); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want int
	}{{"xiaolongxia", 1}, {"xiaolongxia/researcher", 1}, {"xiaolongxia/res", 0}, {"cursor", 1}, {"", 2}} {
		list, err := f.svc.List(ctx, acct, session.ListFilter{AgentPath: tc.path})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != tc.want {
			t.Errorf("agent_path=%q: got %d sessions, want %d", tc.path, len(list), tc.want)
		}
	}
}

func TestIngest_LongAssistantReplyOffersArtifact(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	acct := testutil.NewAccount(t, f.pool)
	req := baseReq("# Go 并发学习笔记\n" + strings.Repeat("goroutine 与 channel 的用法。", 80))
	req.Role = "assistant"
	res, err := f.svc.Ingest(ctx, acct, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Prompts) != 1 || res.Prompts[0].Kind != "artifact" {
		t.Fatalf("prompts = %+v", res.Prompts)
	}
	short := baseReq("短回复")
	short.Role = "assistant"
	res, _ = f.svc.Ingest(ctx, acct, short)
	if len(res.Prompts) != 0 {
		t.Fatalf("short reply must not offer artifact: %+v", res.Prompts)
	}
}

func TestIngest_RejectsForeignProject(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a := testutil.NewAccount(t, f.pool)
	b := testutil.NewAccount(t, f.pool)
	p, err := project.NewStore(f.pool).Create(ctx, a, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	req := baseReq("x")
	req.ProjectID = p.ID.String()
	if _, err := f.svc.Ingest(ctx, b, req); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestIngest_ExternalSessionIDReusesSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	acct := testutil.NewAccount(t, f.pool)

	req := baseReq("first")
	req.ExternalSessionID = "cursor:conv_abc"
	req.Agent = "cursor"
	req.AgentPath = "cursor"
	res1, err := f.svc.Ingest(ctx, acct, req)
	if err != nil {
		t.Fatal(err)
	}
	req2 := baseReq("second")
	req2.ExternalSessionID = "cursor:conv_abc"
	req2.Agent = "cursor"
	req2.AgentPath = "cursor"
	req2.Role = "assistant"
	res2, err := f.svc.Ingest(ctx, acct, req2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.SessionID != res1.SessionID || res2.Seq != 2 {
		t.Fatalf("want same session seq=2, got %+v vs %+v", res1, res2)
	}

	req3 := baseReq("other agent")
	req3.ExternalSessionID = "cursor:conv_abc"
	req3.Agent = "codex"
	req3.AgentPath = "codex"
	res3, err := f.svc.Ingest(ctx, acct, req3)
	if err != nil {
		t.Fatal(err)
	}
	if res3.SessionID == res1.SessionID {
		t.Fatal("different agent must not share external session")
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
