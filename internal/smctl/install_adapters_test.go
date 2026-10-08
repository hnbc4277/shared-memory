package smctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestInstallDeepSeek_WritesKit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dst, err := InstallDeepSeek()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hooks.json", "cordis.snippet.yml", "INSTALL.txt"} {
		p := filepath.Join(dst, name)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) == 0 {
			t.Fatalf("empty %s", name)
		}
	}
	hooks, _ := os.ReadFile(filepath.Join(dst, "hooks.json"))
	if !strings.Contains(string(hooks), "deepseek-harness") {
		t.Fatalf("hooks missing agent: %s", hooks)
	}
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
