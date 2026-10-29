package smctl

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sharedmemory/internal/cred"
)

// InstallHermes copies the Hermes MemoryProvider into ~/.hermes/plugins/memory/shared_memory
// and writes a short setup note. Does not overwrite an existing plugin.yaml with different content
// unless force semantics via identical tree replace of known files.
func InstallHermes() (string, error) {
	src, err := findAdapterDir("hermes")
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, ".hermes", "plugins", "memory", "shared_memory")
	if err := copyTree(src, dst); err != nil {
		return "", err
	}
	if err := writeKeepaliveScripts(dst); err != nil {
		return "", err
	}
	note := filepath.Join(dst, "INSTALL.txt")
	body := hermesInstallNote(credEnvBlock())
	if err := os.WriteFile(note, []byte(body), 0o644); err != nil {
		return "", err
	}
	cfgHint := filepath.Join(home, ".hermes", "shared-memory-config.snippet.yaml")
	_ = os.WriteFile(cfgHint, []byte("memory:\n  provider: shared_memory\n"), 0o644)
	return dst, nil
}

// InstallOpenClaw writes an install kit under ~/.shared-memory/openclaw with
// build/enable instructions and a copy of the adapter package for linking.
func InstallOpenClaw() (string, error) {
	src, err := findAdapterDir("openclaw")
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, ".shared-memory", "openclaw")
	if err := copyTree(src, dst); err != nil {
		return "", err
	}
	if err := writeKeepaliveScripts(dst); err != nil {
		return "", err
	}
	note := filepath.Join(dst, "INSTALL.txt")
	body := openclawInstallNote(dst, credEnvBlock())
	if err := os.WriteFile(note, []byte(body), 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// InstallDoubao copies the Doubao MCP+Skill kit to ~/.shared-memory/doubao
// and writes a personalized mcp.connector.json plus INSTALL.txt.
func InstallDoubao() (string, error) {
	src, err := findAdapterDir("doubao")
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, ".shared-memory", "doubao")
	if err := copyTree(src, dst); err != nil {
		return "", err
	}
	mcp, _ := lookBeside("shared-memory-mcp")
	if mcp == "" {
		mcp, _ = lookBeside("mcp")
	}
	if mcp == "" {
		mcp = "shared-memory-mcp"
	}
	apiURL := "http://127.0.0.1:18765"
	apiKey := "sm_..."
	connID := ""
	if c, err := cred.Resolve(); err == nil {
		if c.APIURL != "" {
			apiURL = c.APIURL
		}
		if c.APIKey != "" {
			apiKey = c.APIKey
		}
		connID = strings.TrimSpace(c.ConnectionID)
	}
	connector := fmt.Sprintf(`{
  "name": "shared-memory",
  "transport": "stdio",
  "command": %q,
  "args": [],
  "env": {
    "SHARED_MEMORY_API_URL": %q,
    "SHARED_MEMORY_API_KEY": %q,
    "SHARED_MEMORY_AGENT": "doubao",
    "SHARED_MEMORY_CONNECTION_ID": %q
  }
}
`, mcp, apiURL, apiKey, connID)
	if err := os.WriteFile(filepath.Join(dst, "mcp.connector.json"), []byte(connector), 0o644); err != nil {
		return "", err
	}
	if err := writeKeepaliveScripts(dst); err != nil {
		return "", err
	}
	skillNote := ""
	if skillDst, err := installDoubaoUserSkill(filepath.Join(dst, "SKILL.md")); err != nil {
		skillNote = "Skill auto-install skipped: " + err.Error() + "\n" +
			"  Manual: copy SKILL.md into Doubao .user_skills/shared-memory/\n"
	} else {
		skillNote = "Skill auto-installed → " + skillDst + "\n" +
			"  Restart 豆包工作 or open a new task if the skill list does not refresh.\n"
		_ = os.WriteFile(filepath.Join(dst, "SKILL_INSTALLED.txt"), []byte(skillDst+"\n"), 0o644)
	}
	note := doubaoInstallNote(dst, credEnvBlock(), skillNote)
	if err := os.WriteFile(filepath.Join(dst, "INSTALL.txt"), []byte(note), 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// doubaoUserSkillsRoots lists known Doubao desktop .user_skills directories (best-effort; layout may change).
func doubaoUserSkillsRoots() []string {
	var roots []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p != "" {
			roots = append(roots, p)
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		add(filepath.Join(local, "Doubao", "User Data", "Default", ".doubao", "agent_mode", "workspace", ".user_skills"))
		add(filepath.Join(local, "Doubao", "User Data", "Default", ".doubaowork", "agent_mode", "workspace", ".user_skills"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "Library", "Application Support", "Doubao", "User Data", "Default", ".doubao", "agent_mode", "workspace", ".user_skills"))
		add(filepath.Join(home, ".doubao", "agent_mode", "workspace", ".user_skills"))
		add(filepath.Join(home, ".doubaowork", "agent_mode", "workspace", ".user_skills"))
	}
	return roots
}

// installDoubaoUserSkill copies SKILL.md into Doubao's .user_skills/shared-memory/.
func installDoubaoUserSkill(skillPath string) (string, error) {
	body, err := os.ReadFile(skillPath)
	if err != nil {
		return "", err
	}
	roots := doubaoUserSkillsRoots()
	if len(roots) == 0 {
		return "", fmt.Errorf("no Doubao .user_skills candidate path on this OS")
	}
	var destRoot string
	for _, root := range roots {
		parent := filepath.Dir(root)
		if st, err := os.Stat(parent); err == nil && st.IsDir() {
			destRoot = root
			break
		}
		grand := filepath.Dir(parent)
		gg := filepath.Dir(grand)
		profile := filepath.Dir(gg)
		if st, err := os.Stat(profile); err == nil && st.IsDir() {
			destRoot = root
			break
		}
	}
	if destRoot == "" {
		destRoot = roots[0]
	}
	skillDir := filepath.Join(destRoot, "shared-memory")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(out, body, 0o644); err != nil {
		return "", err
	}
	return out, nil
}

// InstallDeepSeek writes hooks.json + cordis snippet under ~/.shared-memory/deepseek-harness
// with absolute smhook/mcp paths when resolvable.
func InstallDeepSeek() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(home, ".shared-memory", "deepseek-harness")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	smhook, _ := lookBeside("smhook")
	if smhook == "" {
		smhook = "smhook"
	}
	mcp, _ := lookBeside("shared-memory-mcp")
	if mcp == "" {
		mcp, _ = lookBeside("mcp")
	}
	if mcp == "" {
		mcp = "shared-memory-mcp"
	}
	hooksCmd := deepSeekHookCommand(smhook)
	hooks := fmt.Sprintf(`{
  "version": 1,
  "hooks": {
    "SessionStart": [{"name":"shared-memory","command":%q}],
    "UserPromptSubmit": [{"name":"shared-memory","command":%q}],
    "Stop": [{"name":"shared-memory","command":%q}],
    "SubagentStop": [{"name":"shared-memory","command":%q}]
  }
}
`, hooksCmd, hooksCmd, hooksCmd, hooksCmd)
	if err := os.WriteFile(filepath.Join(dst, "hooks.json"), []byte(hooks), 0o644); err != nil {
		return "", err
	}
	cordis := fmt.Sprintf(`# Generated by smctl install deepseek-harness — merge into your project cordis.yml
plugins:
  - id: mcp-shared-memory
    name: "@deepseek-ai/dsh-mcp-client"
    config:
      serverName: shared-memory
      transport: stdio
      command: %q
      env:
        SHARED_MEMORY_API_KEY: !!js process.env.SHARED_MEMORY_API_KEY
        SHARED_MEMORY_API_URL: !!js 'process.env.SHARED_MEMORY_API_URL || "http://127.0.0.1:18765"'
        SHARED_MEMORY_AGENT: deepseek-harness
        SHARED_MEMORY_CONNECTION_ID: !!js 'process.env.SHARED_MEMORY_CONNECTION_ID || ""'

  - id: hooks-shared-memory
    name: "@deepseek-ai/dsh-hooks-codex"
    config:
      configPath: %q
`, mcp, filepath.ToSlash(filepath.Join(dst, "hooks.json")))
	if err := os.WriteFile(filepath.Join(dst, "cordis.snippet.yml"), []byte(cordis), 0o644); err != nil {
		return "", err
	}
	if err := writeKeepaliveScripts(dst); err != nil {
		return "", err
	}
	note := deepSeekInstallNote(dst, credEnvBlock())
	if err := os.WriteFile(filepath.Join(dst, "INSTALL.txt"), []byte(note), 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

func deepSeekHookCommand(smhook string) string {
	conn := ""
	if c, err := cred.Resolve(); err == nil {
		conn = strings.TrimSpace(c.ConnectionID)
	}
	if runtime.GOOS == "windows" {
		if conn != "" {
			return fmt.Sprintf(
				`cmd /c "set SHARED_MEMORY_HOOK_SURFACE=codex&& set SHARED_MEMORY_AGENT=deepseek-harness&& set SHARED_MEMORY_CONNECTION_ID=%s&& %s"`,
				conn, smhook)
		}
		return fmt.Sprintf(`cmd /c "set SHARED_MEMORY_HOOK_SURFACE=codex&& set SHARED_MEMORY_AGENT=deepseek-harness&& %s"`, smhook)
	}
	if conn != "" {
		return fmt.Sprintf("env SHARED_MEMORY_HOOK_SURFACE=codex SHARED_MEMORY_AGENT=deepseek-harness SHARED_MEMORY_CONNECTION_ID=%s %s",
			shellQuote(conn), shellQuote(smhook))
	}
	return fmt.Sprintf("env SHARED_MEMORY_HOOK_SURFACE=codex SHARED_MEMORY_AGENT=deepseek-harness %s", shellQuote(smhook))
}

func findAdapterDir(name string) (string, error) {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "adapters", name),
			filepath.Join(wd, "..", "adapters", name),
			filepath.Join(wd, "..", "..", "adapters", name),
		)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "adapters", name),
			filepath.Join(dir, "..", "adapters", name),
			filepath.Join(dir, "..", "..", "adapters", name),
		)
	}
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err == nil && st.IsDir() {
			return filepath.Clean(c), nil
		}
	}
	return "", fmt.Errorf("adapters/%s not found (run smctl from the sharedMemory repo, or keep adapters/ beside the binary)", name)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		// skip node_modules / build junk
		base := d.Name()
		if base == "node_modules" || base == "dist" || base == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func credEnvBlock() string {
	c, err := cred.Resolve()
	apiURL := "http://127.0.0.1:18765"
	apiKey := "sm_..."
	if err == nil {
		if c.APIURL != "" {
			apiURL = c.APIURL
		}
		if c.APIKey != "" {
			apiKey = c.APIKey
		}
	}
	block := fmt.Sprintf("SHARED_MEMORY_API_URL=%s\nSHARED_MEMORY_API_KEY=%s\n", apiURL, apiKey)
	if err == nil && strings.TrimSpace(c.ConnectionID) != "" {
		block += fmt.Sprintf("SHARED_MEMORY_CONNECTION_ID=%s\n", c.ConnectionID)
	}
	return block
}

func hermesInstallNote(env string) string {
	return strings.TrimSpace(`
Shared Memory — Hermes install (smctl)

1. Plugin files are in this directory (MemoryProvider).
2. In ~/.hermes/config.yaml set:

memory:
  provider: shared_memory

   (snippet also written to ~/.hermes/shared-memory-config.snippet.yaml)

3. Put credentials in ~/.hermes/.env (or process env):

`+env+`
4. Injection: Hermes prefetch → recall (automatic). Failures: check control console Recall Receipts.
5. Provenance: sync_turn fills skills/plugins/tools/model when Hermes passes them in kwargs.
6. Connection heartbeat (process_online): the plugin pings while loaded.
`+keepaliveInstallNote()) + "\n"
}

func openclawInstallNote(dir, env string) string {
	return strings.TrimSpace(fmt.Sprintf(`
Shared Memory — OpenClaw / 小龙虾 install (smctl)

Kit copied to:
  %s

1. Build:
   cd %s
   npm install
   npm run build

2. Enable plugin id "shared-memory" in OpenClaw (path/link this folder).

3. Environment:

%s
4. Injection: before_prompt_build → prependContext (automatic).
5. agent=xiaolongxia; subagents → agent_path=xiaolongxia/<id>
6. Connection heartbeat (process_online): the adapter pings while loaded.
`+keepaliveInstallNote(), dir, dir, env)) + "\n"
}

func doubaoInstallNote(dir, env, skillNote string) string {
	return strings.TrimSpace(fmt.Sprintf(`
Shared Memory — 豆包 (Doubao) install (smctl) — plan C: MCP + Skill

Files written to:
  %s
  - mcp.connector.json   (paste into 豆包工作 custom MCP connector — still manual)
  - SKILL.md
  - mcp.connector.example.json

Skill:
%s
1. Ensure shared-memory-mcp is on PATH (or edit command in mcp.connector.json).
2. In 豆包工作, add a custom STDIO MCP connector using mcp.connector.json fields (UI only; no public config file).
3. Environment (also embedded in mcp.connector.json when smctl login succeeded):

%s
4. Injection: model-driven via Skill + MCP (not silent hooks). Check Recall Receipts.
5. agent=doubao. Connection heartbeat: MCP ticker while shared-memory-mcp runs.
`+keepaliveInstallNote(), dir, skillNote, env)) + "\n"
}

func deepSeekInstallNote(dir, env string) string {
	return strings.TrimSpace(fmt.Sprintf(`
Shared Memory — DeepSeek Harness install (smctl)

Files written to:
  %s
  - hooks.json          (Codex-dialect smhook, agent=deepseek-harness)
  - cordis.snippet.yml  (merge into project cordis.yml)

1. Ensure smhook and shared-memory-mcp are on PATH (or edit paths in the files).
2. Merge cordis.snippet.yml plugins into your harness cordis.yml.
3. Point dsh-hooks-codex configPath at this hooks.json (already set in the snippet).
4. Environment:

%s
5. Injection: Codex-style additionalContext (automatic) + MCP memory_recall.
6. Connection heartbeat (process_online): MCP ticker while shared-memory-mcp runs.
`+keepaliveInstallNote(), dir, env)) + "\n"
}

// Keep in sync with adapters/shared/sm-connection-heartbeat.{sh,ps1}.
func writeKeepaliveScripts(dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dst, "sm-connection-heartbeat.sh"), []byte(keepaliveSh), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, "sm-connection-heartbeat.ps1"), []byte(keepalivePS1), 0o644)
}

func keepaliveInstallNote() string {
	return strings.TrimSpace(`
   Optional install-side keepalive (client=setup) when the product is not running.
   Scripts in this directory read SHARED_MEMORY_API_URL / SHARED_MEMORY_API_KEY / SHARED_MEMORY_CONNECTION_ID.

   Unix:
     chmod +x sm-connection-heartbeat.sh
     nohup ./sm-connection-heartbeat.sh >/tmp/sm-connection-heartbeat.log 2>&1 &

   Windows:
     powershell -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File .\sm-connection-heartbeat.ps1

   Task Scheduler / systemd --user is optional and not registered automatically.
`) + "\n"
}

const keepaliveSh = `#!/usr/bin/env sh
# Shared Memory connection keepalive (install-side).
# Reads SHARED_MEMORY_API_URL / SHARED_MEMORY_API_KEY / SHARED_MEMORY_CONNECTION_ID.
# Optional SHARED_MEMORY_HEARTBEAT_INTERVAL (seconds, default 30).
set -eu
API_URL="${SHARED_MEMORY_API_URL:-http://127.0.0.1:18765}"
API_KEY="${SHARED_MEMORY_API_KEY:-}"
CONN_ID="${SHARED_MEMORY_CONNECTION_ID:-}"
INTERVAL="${SHARED_MEMORY_HEARTBEAT_INTERVAL:-30}"

if [ -z "$API_KEY" ]; then
  echo "sm-connection-heartbeat: SHARED_MEMORY_API_KEY is required" >&2
  exit 1
fi

API_URL="${API_URL%/}"

post() {
  if [ -n "$CONN_ID" ]; then
    body="{\"client\":\"setup\",\"connection_id\":\"${CONN_ID}\"}"
  else
    body='{"client":"setup"}'
  fi
  curl -sS -o /dev/null \
    -X POST "${API_URL}/v1/agents/connections/heartbeat" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$body" || echo "sm-connection-heartbeat: request failed" >&2
}

while true; do
  post
  sleep "$INTERVAL"
done
`

const keepalivePS1 = `#Requires -Version 5.1
# Shared Memory connection keepalive (install-side).
# Reads SHARED_MEMORY_API_URL / SHARED_MEMORY_API_KEY / SHARED_MEMORY_CONNECTION_ID.
# Optional SHARED_MEMORY_HEARTBEAT_INTERVAL (seconds, default 30).
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Continue'
$ApiUrl = $env:SHARED_MEMORY_API_URL
if ([string]::IsNullOrWhiteSpace($ApiUrl)) { $ApiUrl = 'http://127.0.0.1:18765' }
$ApiKey = $env:SHARED_MEMORY_API_KEY
$ConnectionId = $env:SHARED_MEMORY_CONNECTION_ID
$Interval = 30
if ($env:SHARED_MEMORY_HEARTBEAT_INTERVAL) {
  $parsed = 0
  if ([int]::TryParse($env:SHARED_MEMORY_HEARTBEAT_INTERVAL, [ref]$parsed) -and $parsed -gt 0) {
    $Interval = $parsed
  }
}
if ([string]::IsNullOrWhiteSpace($ApiKey)) {
  Write-Error 'sm-connection-heartbeat: SHARED_MEMORY_API_KEY is required'
  exit 1
}
$ApiUrl = $ApiUrl.TrimEnd('/')
$body = @{ client = 'setup' }
if (-not [string]::IsNullOrWhiteSpace($ConnectionId)) {
  $body.connection_id = $ConnectionId
}
while ($true) {
  try {
    Invoke-RestMethod -Method Post -Uri "$ApiUrl/v1/agents/connections/heartbeat" -Headers @{ Authorization = "Bearer $ApiKey" } -ContentType 'application/json' -Body ($body | ConvertTo-Json)
  } catch {
    Write-Warning $_.Exception.Message
  }
  Start-Sleep -Seconds $Interval
}
`
