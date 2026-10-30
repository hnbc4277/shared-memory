package agents

import (
	_ "embed"
	"strings"
)

//go:embed generic_setup.ps1
var genericSetupTemplate string

// RenderGenericSetupScript fills the thin PowerShell template for non-Cursor agents.
func RenderGenericSetupScript(apiURL, apiKey, connectionID, installAgent string) string {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if apiURL == "" {
		apiURL = "http://127.0.0.1:18080"
	}
	installAgent = strings.TrimSpace(installAgent)
	if installAgent == "" {
		installAgent = "codex"
	}
	r := strings.NewReplacer(
		"__SM_API_URL__", escapePSSingleQuoted(apiURL),
		"__SM_API_KEY__", escapePSSingleQuoted(strings.TrimSpace(apiKey)),
		"__SM_CONNECTION_ID__", escapePSSingleQuoted(strings.TrimSpace(connectionID)),
		"__SM_INSTALL_AGENT__", escapePSSingleQuoted(installAgent),
	)
	out := r.Replace(stripUTF8BOM(genericSetupTemplate))
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\n", "\r\n")
	return out
}

// RenderGenericSetupLauncher returns a double-clickable ASCII .cmd for the sibling generic .ps1.
func RenderGenericSetupLauncher(installAgent string) string {
	_ = installAgent
	return `@echo off
cd /d "%~dp0"
set "PS1=%~dpn0.ps1"
if not exist "%PS1%" (
  echo ERROR: missing "%PS1%"
  echo Keep the .cmd and .ps1 in the same folder with the same base name.
  pause
  exit /b 1
)
echo Shared Memory setup
echo.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%PS1%"
set ERR=%ERRORLEVEL%
echo.
if not "%ERR%"=="0" (
  echo Setup failed, exit code %ERR%
  pause
  exit /b %ERR%
)
echo Done.
pause
`
}
