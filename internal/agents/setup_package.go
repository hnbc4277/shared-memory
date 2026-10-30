package agents

import (
	"strings"
)

// SetupBundleFile documents expected zip layout for HTTP/docs; parsing lives in httpapi.
type SetupBundleFile struct {
	ZipName  string
	DiskHint string
}

var setupScriptAgents = map[string]struct{}{
	"cursor":           {},
	"codex":            {},
	"hermes":           {},
	"openclaw":         {},
	"xiaolongxia":      {},
	"deepseek-harness": {},
	"doubao":           {},
}

// SupportsSetupScript reports whether agent has a one-click setup zip flow.
func SupportsSetupScript(agent string) bool {
	_, ok := setupScriptAgents[strings.ToLower(strings.TrimSpace(agent))]
	return ok
}

// SetupZipBaseName returns an ASCII zip/cmd/ps1 base: setup-{agent}-{12hex}.
func SetupZipBaseName(agent, connectionID string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	if a == "" {
		a = "cursor"
	}
	id := strings.ReplaceAll(strings.TrimSpace(connectionID), "-", "")
	id = strings.ToLower(id)
	if id == "" {
		id = a
	}
	if len(id) > 12 {
		id = id[:12]
	}
	return "setup-" + a + "-" + id
}

// SetupInstallAgent maps display agent id to smctl install target.
func SetupInstallAgent(agent string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	if a == "xiaolongxia" {
		return "openclaw"
	}
	return a
}

// NeedsHookBinaries reports whether setup zip must include hook binaries.
func NeedsHookBinaries(agent string) bool {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "cursor", "codex", "deepseek-harness":
		return true
	default:
		return false
	}
}

// AdapterDirName returns bundled adapter subdirectory name, or "" if none.
func AdapterDirName(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "hermes":
		return "hermes"
	case "openclaw", "xiaolongxia":
		return "openclaw"
	case "doubao":
		return "doubao"
	default:
		return ""
	}
}
