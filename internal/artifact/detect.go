package artifact

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	TypeSkill     = "skill"
	TypeDoc       = "doc"
	TypeNote      = "note"
	TypeChecklist = "checklist"
	TypeOther     = "other"

	// OfferMinRunes is the length at which a long assistant reply is offered as an artifact.
	OfferMinRunes = 800
)

var (
	validTypes  = map[string]bool{TypeSkill: true, TypeDoc: true, TypeNote: true, TypeChecklist: true, TypeOther: true}
	headingRe   = regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	checkboxRe  = regexp.MustCompile(`(?m)^\s*[-*]\s+\[[ xX]\]`)
	skillWordRe = regexp.MustCompile(`(?i)(技能|skill|操作步骤|how[- ]to)`)
	noteWordRe  = regexp.MustCompile(`(?i)(笔记|学习|总结|note|summary)`)
)

func ValidType(t string) bool { return validTypes[t] }

// ShouldOffer reports whether an assistant reply looks like a standalone deliverable.
func ShouldOffer(role, content string, explicit bool) bool {
	if role != "assistant" {
		return false
	}
	if explicit {
		return strings.TrimSpace(content) != ""
	}
	return utf8.RuneCountInString(content) >= OfferMinRunes
}

func GuessType(content string) string {
	head := content
	if utf8.RuneCountInString(head) > 400 {
		head = string([]rune(head)[:400])
	}
	switch {
	case skillWordRe.MatchString(head):
		return TypeSkill
	case len(checkboxRe.FindAllString(content, -1)) >= 3:
		return TypeChecklist
	case noteWordRe.MatchString(head):
		return TypeNote
	case headingRe.MatchString(content):
		return TypeDoc
	default:
		return TypeOther
	}
}

func GuessTitle(content string) string {
	if m := headingRe.FindStringSubmatch(content); m != nil {
		return truncate(strings.TrimSpace(m[1]), 80)
	}
	for _, line := range strings.Split(content, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return truncate(line, 60)
		}
	}
	return "未命名成稿"
}

func Summarize(content string) string {
	var parts []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#>-*"))
		if line != "" {
			parts = append(parts, line)
		}
	}
	return truncate(strings.Join(parts, " "), 200)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
