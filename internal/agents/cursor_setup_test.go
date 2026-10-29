package agents

import (
	"strings"
	"testing"
)

func TestRenderCursorSetupScript(t *testing.T) {
	out := RenderCursorSetupScript(
		"http://127.0.0.1:18080",
		"sm_test_key",
		"11111111-2222-3333-4444-555555555555",
		"http://example.com:28080",
	)
	if strings.Contains(out, "__SM_API_KEY__") || strings.Contains(out, "__SM_CONNECTION_ID__") {
		t.Fatal("placeholders not replaced")
	}
	if !strings.Contains(out, "sm_test_key") {
		t.Fatal("missing api key")
	}
	if !strings.Contains(out, "11111111-2222-3333-4444-555555555555") {
		t.Fatal("missing connection id")
	}
	if !strings.Contains(out, "http://example.com:28080") {
		t.Fatal("missing download base")
	}
	// Sentinel checks must not compare eq to the baked secret (ReplaceAll bug).
	if strings.Contains(out, "$ApiKey -eq 'sm_test_key'") || strings.Contains(out, `$ApiKey -eq "sm_test_key"`) {
		t.Fatal("baked key used as equality sentinel; would clear ApiKey at runtime")
	}
	if !strings.Contains(out, "使用脚本内置凭据") {
		t.Fatal("expected baked-credentials path in template")
	}
	if strings.HasPrefix(out, "\uFEFF") {
		t.Fatal("render must not include BOM; zip writer adds it once")
	}
	if strings.Contains(out, "[CmdletBinding()]") {
		t.Fatal("CmdletBinding before param can break PS 5.1 when a BOM/invisible char precedes it")
	}
	if !strings.HasPrefix(strings.TrimPrefix(out, "\uFEFF"), "#Requires") {
		t.Fatal("script must start with #Requires")
	}
}

func TestPS1FileBytesSingleBOM(t *testing.T) {
	raw := RenderCursorSetupScript("http://x", "sm_k", "11111111-2222-3333-4444-555555555555", "http://x")
	b := PS1FileBytes("\uFEFF" + raw) // even if caller passes a BOM, only one remains
	if len(b) < 4 || b[0] != 0xEF || b[1] != 0xBB || b[2] != 0xBF {
		t.Fatal("missing leading UTF-8 BOM")
	}
	if b[3] == 0xEF && b[4] == 0xBB && b[5] == 0xBF {
		t.Fatal("double BOM")
	}
	if !strings.HasPrefix(string(b[3:]), "#Requires") {
		t.Fatalf("after BOM want #Requires, got %q", string(b[3:20]))
	}
}

func TestParseCursorSetupScriptValues(t *testing.T) {
	oldStyle := `[string]$ApiUrl = "http://example.com:28080",
  [string]$ApiKey = "sm_abc123",
  [string]$ConnectionId = "010779c9-281a-4db5-9e37-1a134a44b182",
  [string]$DownloadBase = "http://example.com:28080",`
	apiURL, apiKey, connID, dl, ok := ParseCursorSetupScriptValues(oldStyle)
	if !ok || apiKey != "sm_abc123" || connID != "010779c9-281a-4db5-9e37-1a134a44b182" {
		t.Fatalf("old style parse failed: ok=%v key=%q id=%q", ok, apiKey, connID)
	}
	if apiURL != "http://example.com:28080" || dl != "http://example.com:28080" {
		t.Fatalf("urls: %q %q", apiURL, dl)
	}
	rendered := RenderCursorSetupScript(apiURL, apiKey, connID, dl)
	_, key2, id2, _, ok2 := ParseCursorSetupScriptValues(rendered)
	if !ok2 || key2 != apiKey || id2 != connID {
		t.Fatal("round-trip parse failed")
	}
}

func TestSetupScriptBaseNameASCII(t *testing.T) {
	base := SetupScriptBaseName("29aed2e3-7886-4f37-818c-331cc3eaa3a4")
	if base != "setup-cursor-29aed2e37886" {
		t.Fatalf("got %q", base)
	}
	for _, r := range base {
		if r > 127 {
			t.Fatalf("non-ascii in %q", base)
		}
	}
}

func TestRenderCursorSetupScript_HeartbeatKeepalive(t *testing.T) {
	out := RenderCursorSetupScript(
		"http://127.0.0.1:18080",
		"sm_test_key",
		"11111111-2222-3333-4444-555555555555",
		"http://127.0.0.1:18080",
	)
	raw := string(PS1FileBytes(out))
	if !strings.Contains(raw, "agents/connections/heartbeat") {
		t.Fatal("missing heartbeat API path in setup script")
	}
	if !strings.Contains(raw, "client") {
		t.Fatal("missing client field in heartbeat template")
	}
	if !strings.Contains(raw, "SharedMemoryHeartbeat-") {
		t.Fatal("missing scheduled task name prefix")
	}
	if !strings.Contains(raw, "sm-connection-heartbeat.ps1") {
		t.Fatal("missing heartbeat script filename")
	}
}

func TestRenderCursorSetupLauncher(t *testing.T) {
	cmd := RenderCursorSetupLauncher()
	if !strings.Contains(cmd, "ExecutionPolicy Bypass") {
		t.Fatal("missing Bypass")
	}
	if !strings.Contains(cmd, `set "PS1=%~dpn0.ps1"`) {
		t.Fatal("missing sibling ps1 resolution")
	}
	if !strings.Contains(cmd, `-File "%PS1%"`) {
		t.Fatal("expected -File launch")
	}
	if strings.Contains(cmd, "%%") {
		t.Fatal("double-escaped percent left in cmd")
	}
	for _, r := range cmd {
		if r > 127 {
			t.Fatal("launcher must be ASCII-only for cmd.exe")
		}
	}
}
