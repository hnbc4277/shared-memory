package smctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sharedmemory/internal/cred"
)

const HookName = "shared-memory"

// InstallCursor merges hooks, MCP snippet, and a rules file.
func InstallCursor(smhookPath, mcpPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cursorDir := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return err
	}
	if smhookPath == "" {
		var err error
		smhookPath, err = lookBeside("smhook")
		if err != nil {
			return fmt.Errorf("smhook path: %w", err)
		}
	}
	if mcpPath == "" {
		mcpPath, _ = lookBeside("mcp")
		if mcpPath == "" {
			mcpPath, _ = lookBeside("shared-memory-mcp")
		}
	}

	hooksPath := filepath.Join(cursorDir, "hooks.json")
	cmd := shellQuote(smhookPath)
	envPrefix := "SHARED_MEMORY_HOOK_SURFACE=cursor "
	if runtime.GOOS == "windows" {
		// Cursor on Windows typically runs via cmd; set env inline.
		cmd = fmt.Sprintf(`cmd /c "set SHARED_MEMORY_HOOK_SURFACE=cursor&& %s"`, smhookPath)
		envPrefix = ""
	} else {
		cmd = envPrefix + cmd
	}
	events := []string{"beforeSubmitPrompt", "afterAgentResponse", "subagentStop", "sessionStart"}
	if err := mergeHooksJSON(hooksPath, events, cmd); err != nil {
		return err
	}

	mcpFile := filepath.Join(cursorDir, "mcp.json")
	if err := mergeMCPJSON(mcpFile, mcpPath); err != nil {
		return err
	}

	rulesDir := filepath.Join(cursorDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return err
	}
	rule := `---
description: Shared Memory — always recall before reinventing context
globs:
alwaysApply: true
---

# Shared Memory（强制召回）

Cursor 无法可靠地把 hook 返回的 additional_context 注入模型，因此你必须主动调用 MCP：

1. **每个非琐碎任务开始时**，先调用 ` + "`memory_recall_status`" + `（带上 external_session_id，若可知）：
   - 若返回 ` + "`need_recall: true`" + `，必须先调用 ` + "`memory_recall`" + ` 再继续。
2. 调用 ` + "`memory_recall`" + `：
   - query：用当前用户目标的一句话摘要（中文或英文均可）
   - 若用户提到了具体项目，带上对应 project_id（如果已知）
   - 尽量带上 external_session_id，便于 Receipt 关联
3. 把返回的 ` + "`context`" + `（画像、项目摘要、相关事实、知识库片段）当作已知背景；**不要**让用户重复已经记住的偏好、决策与项目事实。
4. 需要把规范/笔记加入知识库时，用 ` + "`kb_add`" + `（text 或 artifact_id/attachment_id）；**同意保存成稿不等于进知识库**。
5. 重要结论、用户明确偏好、项目决策，在回复后可通过 ` + "`memory_ingest`" + ` 写入（role=assistant 或 user），并填齐 skills/plugins/tools（没有则传 []）。
6. 子智能体任务：agent_path 使用 ` + "`cursor/<subagent_type>`" + `。
7. 绝不要在对话或工具参数里打印 API Key、口令或 vault 明文。
8. 若 memory_recall 失败或超时，正常继续任务，不要卡死；控制台「密钥」与 ` + "`smctl status`" + ` 可查看最近 Recall Receipt。
`
	return os.WriteFile(filepath.Join(rulesDir, "shared-memory.mdc"), []byte(rule), 0o644)
}

// InstallCodex merges ~/.codex/hooks.json and documents MCP in config.
func InstallCodex(smhookPath, mcpPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return err
	}
	if smhookPath == "" {
		var err error
		smhookPath, err = lookBeside("smhook")
		if err != nil {
			return fmt.Errorf("smhook path: %w", err)
		}
	}
	if mcpPath == "" {
		mcpPath, _ = lookBeside("mcp")
	}

	cmd := shellQuote(smhookPath)
	if runtime.GOOS == "windows" {
		cmd = fmt.Sprintf(`cmd /c "set SHARED_MEMORY_HOOK_SURFACE=codex&& %s"`, smhookPath)
	} else {
		cmd = "SHARED_MEMORY_HOOK_SURFACE=codex " + cmd
	}
	events := []string{"SessionStart", "UserPromptSubmit", "Stop", "SubagentStop"}
	hooksPath := filepath.Join(codexDir, "hooks.json")
	if err := mergeHooksJSON(hooksPath, events, cmd); err != nil {
		return err
	}

	// Document MCP + optional config.toml [hooks] pointer.
	snippet := filepath.Join(codexDir, "shared-memory-mcp.json")
	mcpCfg := map[string]any{
		"mcpServers": map[string]any{
			HookName: mcpServerEntry(mcpPath),
		},
	}
	raw, _ := json.MarshalIndent(mcpCfg, "", "  ")
	if err := os.WriteFile(snippet, append(raw, '\n'), 0o644); err != nil {
		return err
	}

	cfgPath := filepath.Join(codexDir, "config.toml")
	note := "\n# shared-memory: also see hooks.json and shared-memory-mcp.json\n# Merge mcpServers.shared-memory into your Codex MCP config.\n"
	if _, err := os.Stat(cfgPath); err == nil {
		b, _ := os.ReadFile(cfgPath)
		if !strings.Contains(string(b), "shared-memory") {
			f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, _ = f.WriteString(note)
			_ = f.Close()
		}
	} else {
		_ = os.WriteFile(cfgPath, []byte("# Codex config\n"+note), 0o644)
	}
	return nil
}

func mergeHooksJSON(path string, events []string, command string) error {
	root := map[string]any{"version": 1, "hooks": map[string]any{}}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &root)
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		root["hooks"] = hooks
	}
	entry := map[string]any{"command": command, "name": HookName}
	for _, ev := range events {
		list := asMapSlice(hooks[ev])
		list = upsertNamed(list, entry)
		hooks[ev] = list
	}
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func mergeMCPJSON(path, mcpBinary string) error {
	root := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &root)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		root["mcpServers"] = servers
	}
	servers[HookName] = mcpServerEntry(mcpBinary)
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func mcpServerEntry(mcpBinary string) map[string]any {
	c, _ := cred.Resolve()
	apiURL := cred.DefaultAPIURL
	apiKey := ""
	if c.APIURL != "" {
		apiURL = c.APIURL
	}
	if c.APIKey != "" {
		apiKey = c.APIKey
	}
	cmd := mcpBinary
	if cmd == "" {
		cmd = "shared-memory-mcp"
	}
	return map[string]any{
		"command": cmd,
		"env": map[string]string{
			"SHARED_MEMORY_API_URL": apiURL,
			"SHARED_MEMORY_API_KEY": apiKey,
			"SHARED_MEMORY_AGENT":   "cursor",
		},
	}
}

func asMapSlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case nil:
		return nil
	default:
		return []any{t}
	}
}

func upsertNamed(list []any, entry map[string]any) []any {
	name, _ := entry["name"].(string)
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if n, _ := m["name"].(string); n == name {
			list[i] = entry
			return list
		}
	}
	return append(list, entry)
}

func lookBeside(base string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(dir, base),
		filepath.Join(dir, base+".exe"),
	}
	// Also try go-run style from repo.
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "bin", base),
			filepath.Join(wd, "bin", base+".exe"),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s not found beside %s", base, exe)
}

func shellQuote(path string) string {
	if strings.ContainsAny(path, " \t\"'") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}
