package smctl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrintRecentReceipts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/recall/receipts" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"queried_at": time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC),
			"agent":      "cursor",
			"hit_count":  2,
			"degraded":   false,
			"source":     "mcp",
		}})
	}))
	defer srv.Close()

	var out strings.Builder
	printRecentReceipts(context.Background(), &out, srv.URL, "k")
	s := out.String()
	if !strings.Contains(s, "recall_receipts (recent)") || !strings.Contains(s, "agent=cursor") || !strings.Contains(s, "source=mcp") {
		t.Fatalf("%s", s)
	}
}
