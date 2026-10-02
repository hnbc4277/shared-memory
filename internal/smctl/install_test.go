package smctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeHooksIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	// Pre-existing unrelated hook.
	_ = os.WriteFile(path, []byte(`{
  "version": 1,
  "hooks": {
    "beforeSubmitPrompt": [{"name":"other","command":"echo hi"}]
  }
}`), 0o644)

	if err := mergeHooksJSON(path, []string{"beforeSubmitPrompt", "sessionStart"}, "smhook"); err != nil {
		t.Fatal(err)
	}
	if err := mergeHooksJSON(path, []string{"beforeSubmitPrompt", "sessionStart"}, "smhook-v2"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	hooks := root["hooks"].(map[string]any)
	list := hooks["beforeSubmitPrompt"].([]any)
	if len(list) != 2 {
		t.Fatalf("expected other+shared-memory, got %d: %s", len(list), raw)
	}
	names := map[string]string{}
	for _, item := range list {
		m := item.(map[string]any)
		names[m["name"].(string)] = m["command"].(string)
	}
	if names["other"] != "echo hi" {
		t.Fatalf("other overwritten: %v", names)
	}
	if names[HookName] != "smhook-v2" {
		t.Fatalf("shared-memory not updated: %v", names)
	}
	if _, ok := hooks["sessionStart"]; !ok {
		t.Fatal("missing sessionStart")
	}
}

func TestMergeMCPJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHARED_MEMORY_HOME", dir)
	path := filepath.Join(dir, "mcp.json")
	_ = os.WriteFile(path, []byte(`{"mcpServers":{"keep":{"command":"x"}}}`), 0o644)
	if err := mergeMCPJSON(path, "/bin/mcp"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var root map[string]any
	_ = json.Unmarshal(raw, &root)
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["keep"]; !ok {
		t.Fatal("lost keep")
	}
	if _, ok := servers[HookName]; !ok {
		t.Fatal("missing shared-memory")
	}
}
