package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/attachment"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/testutil"
)

func kbTestServer(t *testing.T) (*httptest.Server, uuid.UUID, string, *kb.Store, *artifact.Store, *attachment.Service) {
	t.Helper()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	authStore := auth.NewStore(pool)
	key, err := authStore.CreateAPIKey(context.Background(), acct, "kb")
	if err != nil {
		t.Fatal(err)
	}
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	arts := artifact.NewStore(pool, objects)
	atts := &attachment.Service{Pool: pool, Objects: objects, Policies: policy.NewStore(pool), Projects: project.NewStore(pool), Queue: rec}
	store := kb.NewStore(pool, objects, rec, nil).WithSources(arts, atts)
	h := httpapi.NewRouter(httpapi.Deps{
		Auth:        authStore,
		Artifacts:   arts,
		Attachments: atts,
		KB:          store,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, acct, key, store, arts, atts
}

func kbAuth(req *http.Request, key string) {
	req.Header.Set("Authorization", "Bearer "+key)
}

func TestKB_UploadListGetDeleteReparse(t *testing.T) {
	srv, _, key, _, _, _ := kbTestServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "guide.md")
	if err != nil {
		t.Fatal(err)
	}
	body := "# hello kb\n\n" + uuid.NewString()
	if _, err := io.WriteString(part, body); err != nil {
		t.Fatal(err)
	}
	_ = mw.WriteField("title", "Guide")
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents", &buf)
	kbAuth(req, key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("upload status=%d body=%s", res.StatusCode, b)
	}
	var created struct {
		Document kb.Document `json:"document"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Document.ID == uuid.Nil || created.Document.Status != kb.StatusPending {
		t.Fatalf("created = %+v", created.Document)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/kb/documents", nil)
	kbAuth(req, key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d", res.StatusCode)
	}
	var list struct {
		Documents []kb.Document `json:"documents"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range list.Documents {
		if d.ID == created.Document.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("list missing document")
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/kb/documents/"+created.Document.ID.String(), nil)
	kbAuth(req, key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d", res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents/"+created.Document.ID.String()+"/reparse", nil)
	kbAuth(req, key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("reparse status=%d body=%s", res.StatusCode, b)
	}

	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/v1/kb/documents/"+created.Document.ID.String(), nil)
	kbAuth(req, key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("delete status=%d body=%s", res.StatusCode, b)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/kb/documents/"+created.Document.ID.String(), nil)
	kbAuth(req, key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status=%d", res.StatusCode)
	}
}

func TestKB_PromoteArtifactJSON(t *testing.T) {
	srv, acct, key, _, arts, _ := kbTestServer(t)
	ctx := context.Background()
	detail, err := arts.Create(ctx, acct, artifact.Input{
		Title: "From API",
		Type:  "doc",
		Body:  "# api promote\n\n" + uuid.NewString(),
	}, artifact.Source{AgentPath: "user/manual"})
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{"artifact_id": detail.Artifact.ID.String()})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents", bytes.NewReader(payload))
	kbAuth(req, key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("promote status=%d body=%s", res.StatusCode, b)
	}
	var created struct {
		Document kb.Document `json:"document"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Document.SourceType != kb.SourceArtifact {
		t.Fatalf("source = %q", created.Document.SourceType)
	}
}

func TestKB_PromoteOtherAccountArtifact_NotFound(t *testing.T) {
	srv, _, key, _, arts, _ := kbTestServer(t)
	ctx := context.Background()
	other := testutil.NewAccount(t, testutil.TestPool(t))
	detail, err := arts.Create(ctx, other, artifact.Input{
		Title: "Not yours",
		Type:  "doc",
		Body:  "# private\n\n" + uuid.NewString(),
	}, artifact.Source{AgentPath: "user/manual"})
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{"artifact_id": detail.Artifact.ID.String()})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents", bytes.NewReader(payload))
	kbAuth(req, key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d body=%s, want 404", res.StatusCode, b)
	}
}

func TestKB_PromoteBothIDs_BadRequest(t *testing.T) {
	srv, _, key, _, _, _ := kbTestServer(t)
	payload, _ := json.Marshal(map[string]any{
		"artifact_id":   uuid.NewString(),
		"attachment_id": uuid.NewString(),
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents", bytes.NewReader(payload))
	kbAuth(req, key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d body=%s, want 400", res.StatusCode, b)
	}
}

func TestKB_UploadUnsupportedMIME(t *testing.T) {
	srv, _, key, _, _, _ := kbTestServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="pic.png"`)
	h.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("\x89PNG\r\n\x1a\nfake")); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents", &buf)
	kbAuth(req, key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d body=%s, want 415", res.StatusCode, b)
	}
}

func TestKB_PromoteAttachmentJSON(t *testing.T) {
	srv, acct, key, _, _, atts := kbTestServer(t)
	ctx := context.Background()
	a, _, err := atts.Upload(ctx, acct, attachment.Upload{
		Filename: "a.md",
		MIME:     "text/markdown",
		Body:     strings.NewReader("# att\n\n" + uuid.NewString()),
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{"attachment_id": a.ID.String()})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/kb/documents", bytes.NewReader(payload))
	kbAuth(req, key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("promote status=%d body=%s", res.StatusCode, b)
	}
}
