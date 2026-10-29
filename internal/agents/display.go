package agents

import "strings"

func DisplayName(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "cursor":
		return "Cursor"
	case "codex":
		return "Codex"
	case "xiaolongxia":
		return "小龙虾"
	case "hermes":
		return "爱马仕"
	case "deepseek-harness", "deepseek_harness":
		return "DeepSeek Harness"
	case "openclaw":
		return "OpenClaw"
	case "doubao":
		return "豆包"
	default:
		if agent == "" {
			return "未知"
		}
		return agent
	}
}
