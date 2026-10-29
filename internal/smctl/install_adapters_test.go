package smctl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFindAdapterDir_FromWD(t *testing.T) {
	root := t.TempDir()
	hermes := filepath.Join(root, "adapters", "hermes")
	if err := os.MkdirAll(hermes, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(hermes, "plugin.yaml"), []byte("name: shared_memory\n"), 0o644)
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	got, err := findAdapterDir("hermes")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(hermes) {
		t.Fatalf("got %q want %q", got, hermes)
	}
}

func TestInstallDoubao_WritesKit(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "adapters", "doubao")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: shared-memory\n---\n"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "mcp.connector.example.json"), []byte(`{"name":"shared-memory"}`+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "README.md"), []byte("# doubao\n"), 0o644)
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dst, err := InstallDoubao()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SKILL.md", "mcp.connector.example.json", "mcp.connector.json", "INSTALL.txt", "sm-connection-heartbeat.sh", "sm-connection-heartbeat.ps1"} {
		b, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty %s", name)
		}
	}
	conn, _ := os.ReadFile(filepath.Join(dst, "mcp.connector.json"))
	if !strings.Contains(string(conn), `"SHARED_MEMORY_AGENT": "doubao"`) {
		t.Fatalf("connector missing agent: %s", conn)
	}
	note, _ := os.ReadFile(filepath.Join(dst, "INSTALL.txt"))
	if !strings.Contains(string(note), "SKILL.md") || !strings.Contains(string(note), "sm-connection-heartbeat") {
		t.Fatalf("INSTALL.txt incomplete: %s", note)
	}
}

func TestInstallDeepSeek_WritesKit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dst, err := InstallDeepSeek()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hooks.json", "cordis.snippet.yml", "INSTALL.txt", "sm-connection-heartbeat.sh", "sm-connection-heartbeat.ps1"} {
		p := filepath.Join(dst, name)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty %s", name)
		}
	}
	note, _ := os.ReadFile(filepath.Join(dst, "INSTALL.txt"))
	if !strings.Contains(string(note), "sm-connection-heartbeat") {
		t.Fatalf("INSTALL.txt missing keepalive note: %s", note)
	}
	sh, _ := os.ReadFile(filepath.Join(dst, "sm-connection-heartbeat.sh"))
	if !strings.Contains(string(sh), "/v1/agents/connections/heartbeat") {
		t.Fatalf("sh missing heartbeat path: %s", sh)
	}
	hooks, _ := os.ReadFile(filepath.Join(dst, "hooks.json"))
	if !strings.Contains(string(hooks), "deepseek-harness") {
		t.Fatalf("hooks missing agent: %s", hooks)
	}
	cordis, _ := os.ReadFile(filepath.Join(dst, "cordis.snippet.yml"))
	if !strings.Contains(string(cordis), "SHARED_MEMORY_CONNECTION_ID") {
		t.Fatalf("cordis missing CONNECTION_ID: %s", cordis)
	}
}

func TestInstallHermes_WritesKeepalive(t *testing.T) {
	root := t.TempDir()
	hermes := filepath.Join(root, "adapters", "hermes")
	if err := os.MkdirAll(hermes, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(hermes, "plugin.yaml"), []byte("name: shared_memory\n"), 0o644)
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dst, err := InstallHermes()
	if err != nil {
		t.Fatal(err)
	}
	assertKeepaliveKit(t, dst)
}

func TestInstallOpenClaw_WritesKeepalive(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "adapters", "openclaw")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(src, "openclaw.plugin.json"), []byte(`{"id":"shared-memory"}`+"\n"), 0o644)
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dst, err := InstallOpenClaw()
	if err != nil {
		t.Fatal(err)
	}
	assertKeepaliveKit(t, dst)
}

func assertKeepaliveKit(t *testing.T, dst string) {
	t.Helper()
	for _, name := range []string{"INSTALL.txt", "sm-connection-heartbeat.sh", "sm-connection-heartbeat.ps1"} {
		b, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty %s", name)
		}
		if name == "INSTALL.txt" && !strings.Contains(string(b), "sm-connection-heartbeat") {
			t.Fatalf("INSTALL.txt missing keepalive: %s", b)
		}
		if strings.HasPrefix(name, "sm-connection-heartbeat") && !strings.Contains(string(b), "/v1/agents/connections/heartbeat") {
			t.Fatalf("%s missing heartbeat path", name)
		}
	}
}

func TestKeepalivePS1_PostsHeartbeat(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell keepalive smoke is Windows-only")
	}
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.Store(r.URL.Path + "\n" + r.Header.Get("Authorization") + "\n" + string(body))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	if err := writeKeepaliveScripts(dir); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "sm-connection-heartbeat.ps1")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script)
	cmd.Env = append(os.Environ(),
		"SHARED_MEMORY_API_URL="+srv.URL,
		"SHARED_MEMORY_API_KEY=sm_test",
		"SHARED_MEMORY_CONNECTION_ID=conn-1",
		"SHARED_MEMORY_HEARTBEAT_INTERVAL=1",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		v, _ := got.Load().(string)
		if strings.Contains(v, "/v1/agents/connections/heartbeat") && strings.Contains(v, "setup") && strings.Contains(v, "conn-1") {
			if !strings.Contains(v, "Bearer sm_test") {
				t.Fatalf("missing bearer: %s", v)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no keepalive heartbeat POST, last=%v", got.Load())
}

func TestCopyTree_SkipsNodeModules(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(src, "node_modules", "pkg"), 0o755)
	_ = os.WriteFile(filepath.Join(src, "node_modules", "pkg", "x"), []byte("y"), 0o644)
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Fatal("node_modules should be skipped")
	}
}
