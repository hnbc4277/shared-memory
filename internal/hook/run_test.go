package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeAPI struct {
	ingests []map[string]any
	context string
	err     error
}

func (f *fakeAPI) Ingest(ctx context.Context, body map[string]any) error {
	f.ingests = append(f.ingests, body)
	return f.err
}

func (f *fakeAPI) Recall(ctx context.Context, query string) (string, error) {
	return f.context, f.err
}

func TestFixtures(t *testing.T) {
	dir := filepath.Join("testdata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".in.json") {
			continue
		}
		base := strings.TrimSuffix(name, ".in.json")
		t.Run(base, func(t *testing.T) {
			inRaw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			wantRaw, err := os.ReadFile(filepath.Join(dir, base+".out.json"))
			if err != nil {
				t.Fatal(err)
			}
			metaRaw, _ := os.ReadFile(filepath.Join(dir, base+".meta.json"))
			surface := "cursor"
			api := &fakeAPI{context: "RECALL_CTX"}
			if len(metaRaw) > 0 {
				var meta struct {
					Surface string `json:"surface"`
					Context string `json:"context"`
				}
				_ = json.Unmarshal(metaRaw, &meta)
				if meta.Surface != "" {
					surface = meta.Surface
				}
				if meta.Context != "" {
					api.context = meta.Context
				}
			}
			var out bytes.Buffer
			if err := Run(context.Background(), surface, api, bytes.NewReader(inRaw), &out); err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("stdout not json: %s", out.String())
			}
			if err := json.Unmarshal(wantRaw, &want); err != nil {
				t.Fatal(err)
			}
			gotB, _ := json.Marshal(got)
			wantB, _ := json.Marshal(want)
			if string(gotB) != string(wantB) {
				t.Fatalf("stdout\ngot  %s\nwant %s", gotB, wantB)
			}
			assertRaw, _ := os.ReadFile(filepath.Join(dir, base+".assert.json"))
			if len(assertRaw) > 0 {
				var as struct {
					IngestCount int               `json:"ingest_count"`
					Ingest      map[string]string `json:"ingest"`
				}
				_ = json.Unmarshal(assertRaw, &as)
				if as.IngestCount > 0 && len(api.ingests) != as.IngestCount {
					t.Fatalf("ingests=%d want %d (%v)", len(api.ingests), as.IngestCount, api.ingests)
				}
				if as.Ingest != nil {
					if len(api.ingests) == 0 {
						t.Fatal("expected ingest")
					}
					last := api.ingests[len(api.ingests)-1]
					for k, v := range as.Ingest {
						if got, _ := last[k].(string); got != v {
							t.Fatalf("ingest[%s]=%v want %q in %+v", k, last[k], v, last)
						}
					}
				}
			}
			// Never leak secrets into stdout.
			if strings.Contains(out.String(), "api_key") || strings.Contains(out.String(), "Bearer ") {
				t.Fatalf("secret-like token in stdout: %s", out.String())
			}
		})
	}
}

func TestTruncateContext(t *testing.T) {
	s := strings.Repeat("a", MaxContextChars+10)
	if got := TruncateContext(s); len(got) != MaxContextChars {
		t.Fatalf("len=%d", len(got))
	}
}
