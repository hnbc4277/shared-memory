package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// withWebStatic optionally serves a Vite build (web/dist) for same-origin deployments.
// API routes (/v1/*, /healthz, /downloads/*) always hit the inner mux.
func withWebStatic(root string, api http.Handler) http.Handler {
	root = strings.TrimSpace(root)
	if root == "" {
		return api
	}
	root = filepath.Clean(root)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := req.URL.Path
		if p == "/healthz" || strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/downloads/") {
			api.ServeHTTP(w, req)
			return
		}
		rel := strings.TrimPrefix(p, "/")
		if rel == "" {
			http.ServeFile(w, req, filepath.Join(root, "index.html"))
			return
		}
		target := filepath.Join(root, filepath.Clean(rel))
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) && target != root {
			http.NotFound(w, req)
			return
		}
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			http.ServeFile(w, req, target)
			return
		}
		http.ServeFile(w, req, filepath.Join(root, "index.html"))
	})
}

func (r *Router) handlePublicDownload(w http.ResponseWriter, req *http.Request) {
	name := filepath.Base(req.PathValue("name"))
	switch name {
	case "smhook.exe", "shared-memory-mcp.exe", "setup-cursor.ps1":
	default:
		http.NotFound(w, req)
		return
	}
	dir := strings.TrimSpace(r.DownloadsDir)
	if dir == "" {
		http.NotFound(w, req)
		return
	}
	target := filepath.Join(dir, name)
	if st, err := os.Stat(target); err != nil || st.IsDir() {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, req, target)
}
