package semantic

import (
	"context"
	"regexp"
	"strings"
)

const (
	KindEpisodeSummary = "episode_summary"
	KindRunningSummary = "running_summary"
	KindFact           = "fact"
	KindDecision       = "decision"
	KindPreference     = "preference"
	KindEntity         = "entity"
	KindAttachmentText = "attachment_text"

	ScopeGlobal  = "global"
	ScopeProject = "project"
)

type Extracted struct {
	Kind  string
	Scope string
	Body  string
}

// Deriver turns (already redacted) message content into semantic memories.
// The heuristic implementation keeps the platform usable without an LLM key;
// an LLM-backed Deriver can replace it behind the same interface.
type Deriver interface {
	Summarize(ctx context.Context, content string) (string, error)
	Extract(ctx context.Context, role, content string, hasProject bool) ([]Extracted, error)
}

type Heuristic struct{}

var (
	sentenceSplit = regexp.MustCompile(`[。！？!?\n]+|\.\s+`)
	spaceRe       = regexp.MustCompile(`\s+`)
	preferenceRe  = regexp.MustCompile(`(?i)(我(更)?喜欢|我(更)?偏好|我习惯|我希望|我倾向|不要|别再|请总是|请始终|\bi prefer\b|\bi like\b|\balways\b|\bnever\b)`)
	decisionRe    = regexp.MustCompile(`(?i)(决定|选定|选用|采用|确定使用|定下来|改为|统一用|\bdecided\b|\bwe will use\b|\bgo with\b)`)
	factRe        = regexp.MustCompile(`(?i)(项目(使用|用的是|基于)|技术栈|端口|部署在|数据库是|\bwe use\b|\buses\b|\bis built on\b)`)
	entityRe      = regexp.MustCompile("`([A-Za-z0-9_./-]{3,64})`")
)

func Collapse(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (Heuristic) Summarize(_ context.Context, content string) (string, error) {
	return Truncate(Collapse(content), 280), nil
}

func (Heuristic) Extract(_ context.Context, role, content string, hasProject bool) ([]Extracted, error) {
	projectScope := ScopeGlobal
	if hasProject {
		projectScope = ScopeProject
	}
	var out []Extracted
	seen := map[string]bool{}
	add := func(kind, scope, body string) {
		body = Truncate(Collapse(body), 300)
		key := kind + "|" + body
		if body == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Extracted{Kind: kind, Scope: scope, Body: body})
	}
	for _, s := range sentenceSplit.Split(content, -1) {
		s = strings.TrimSpace(s)
		if len([]rune(s)) < 4 {
			continue
		}
		switch {
		case role == "user" && preferenceRe.MatchString(s):
			add(KindPreference, ScopeGlobal, s)
		case decisionRe.MatchString(s):
			add(KindDecision, projectScope, s)
		case factRe.MatchString(s):
			add(KindFact, projectScope, s)
		}
	}
	for _, m := range entityRe.FindAllStringSubmatch(content, 10) {
		add(KindEntity, projectScope, m[1])
	}
	return out, nil
}
