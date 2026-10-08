package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sharedmemory/internal/worker"
)

func TestVisionClient_Describe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "屏幕截图，含代码编辑器"}},
			},
		})
	}))
	defer srv.Close()
	v := worker.NewVisionClient(srv.URL+"/v1", "k", "m")
	got, err := v.Describe(context.Background(), "image/png", []byte{0x89, 0x50, 0x4e, 0x47})
	if err != nil || !strings.Contains(got, "代码编辑器") {
		t.Fatalf("%v %q", err, got)
	}
}

func TestExtract_UsesVisionWhenAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "一张红色方块"}},
			},
		})
	}))
	defer srv.Close()

	// minimal 1x1 png
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x05, 0xfe, 0xd4, 0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	e := worker.NewExtractor("")
	e.Vision = worker.NewVisionClient(srv.URL+"/v1", "k", "m")
	got, err := e.Extract(worker.WithCloudVision(context.Background(), true), "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "视觉描述：一张红色方块") {
		t.Fatalf("got %q", got)
	}
	gotOff, _ := e.Extract(context.Background(), "image/png", png)
	if strings.Contains(gotOff, "视觉描述") {
		t.Fatalf("vision should be off without context flag: %q", gotOff)
	}
}
