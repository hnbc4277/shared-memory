package agents

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed cursor_setup.ps1
var cursorSetupTemplate string

var (
	reSetupAPIURL = regexp.MustCompile(`(?m)(?:\$ApiUrl\s*=\s*'([^']*)'|\[string\]\$ApiUrl\s*=\s*"([^"]*)")`)
	reSetupAPIKey = regexp.MustCompile(`(?m)(?:\$ApiKey\s*=\s*'([^']*)'|\[string\]\$ApiKey\s*=\s*"([^"]*)")`)
	reSetupConnID = regexp.MustCompile(`(?m)(?:\$ConnectionId\s*=\s*'([^']*)'|\[string\]\$ConnectionId\s*=\s*"([^"]*)")`)
	reSetupDLBase = regexp.MustCompile(`(?m)(?:\$DownloadBase\s*=\s*'([^']*)'|\[string\]\$DownloadBase\s*=\s*"([^"]*)")`)
)

// RenderCursorSetupScript fills the one-click PowerShell template with connection credentials.
// apiKey and connectionID are baked in at connection-create time for later download.
func RenderCursorSetupScript(apiURL, apiKey, connectionID, downloadBase string) string {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:18080"
	}
	downloadBase = strings.TrimRight(strings.TrimSpace(downloadBase), "/")
	if downloadBase == "" {
		downloadBase = apiURL
	}
	r := strings.NewReplacer(
		"__SM_API_URL__", escapePSSingleQuoted(apiURL),
		"__SM_API_KEY__", escapePSSingleQuoted(strings.TrimSpace(apiKey)),
		"__SM_CONNECTION_ID__", escapePSSingleQuoted(strings.TrimSpace(connectionID)),
		"__SM_DOWNLOAD_BASE__", escapePSSingleQuoted(downloadBase),
	)
	out := r.Replace(stripUTF8BOM(cursorSetupTemplate))
	// PowerShell on Windows is happiest with CRLF; keep a single UTF-8 BOM only at zip write time.
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\n", "\r\n")
	return out
}

func stripUTF8BOM(s string) string {
	return strings.TrimPrefix(s, "\uFEFF")
}

// PS1FileBytes returns the script bytes with exactly one UTF-8 BOM for Windows PowerShell 5.1.
func PS1FileBytes(script string) []byte {
	script = stripUTF8BOM(script)
	return append([]byte{0xEF, 0xBB, 0xBF}, []byte(script)...)
}

// ParseCursorSetupScriptValues extracts baked credentials from a previously rendered script
// (old double-quoted param defaults or new single-quoted assignments).
func ParseCursorSetupScriptValues(script string) (apiURL, apiKey, connectionID, downloadBase string, ok bool) {
	apiURL = firstGroup(reSetupAPIURL.FindStringSubmatch(script))
	apiKey = firstGroup(reSetupAPIKey.FindStringSubmatch(script))
	connectionID = firstGroup(reSetupConnID.FindStringSubmatch(script))
	downloadBase = firstGroup(reSetupDLBase.FindStringSubmatch(script))
	if strings.Contains(apiKey, "__SM_") || strings.Contains(connectionID, "__SM_") {
		return "", "", "", "", false
	}
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(connectionID) == "" {
		return "", "", "", "", false
	}
	return apiURL, apiKey, connectionID, downloadBase, true
}

func firstGroup(m []string) string {
	if len(m) < 2 {
		return ""
	}
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}

// SetupScriptBaseName returns an ASCII-only base name for zip/cmd/ps1.
// Chinese labels break cmd.exe encoding on many Windows installs, so we use the connection id.
func SetupScriptBaseName(connectionID string) string {
	id := strings.TrimSpace(connectionID)
	id = strings.ReplaceAll(id, "-", "")
	if id == "" {
		id = "cursor"
	}
	if len(id) > 12 {
		id = id[:12]
	}
	return "setup-cursor-" + strings.ToLower(id)
}

// RenderCursorSetupLauncher returns a double-clickable ASCII .cmd that runs the sibling .ps1
// (same base name: %~dpn0.ps1) with ExecutionPolicy Bypass.
func RenderCursorSetupLauncher() string {
	// Keep this file pure ASCII. UTF-8 .cmd without BOM breaks on Chinese Windows
	// (symptoms: "'01' is not recognized", garbled ps1 path, etc.).
	return `@echo off
cd /d "%~dp0"
set "PS1=%~dpn0.ps1"
if not exist "%PS1%" (
  echo ERROR: missing "%PS1%"
  echo Keep the .cmd and .ps1 in the same folder with the same base name.
  pause
  exit /b 1
)
echo Shared Memory Cursor setup
echo.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%PS1%"
set ERR=%ERRORLEVEL%
echo.
if not "%ERR%"=="0" (
  echo Setup failed, exit code %ERR%
  pause
  exit /b %ERR%
)
echo Done. Fully quit Cursor, reopen it, then send a test message.
pause
`
}

// escapePSSingleQuoted prepares a value for a PowerShell single-quoted string ('...').
// Only single quotes need doubling; no $ / ` expansion inside single quotes.
func escapePSSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
