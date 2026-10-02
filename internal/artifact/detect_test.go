package artifact_test

import (
	"strings"
	"testing"

	"sharedmemory/internal/artifact"
)

func TestShouldOffer(t *testing.T) {
	long := strings.Repeat("内容", artifact.OfferMinRunes)
	if !artifact.ShouldOffer("assistant", long, false) {
		t.Fatal("long assistant reply must be offered")
	}
	if artifact.ShouldOffer("user", long, false) {
		t.Fatal("user messages are never offered")
	}
	if artifact.ShouldOffer("assistant", "short", false) {
		t.Fatal("short reply must not be offered")
	}
	if !artifact.ShouldOffer("assistant", "short", true) {
		t.Fatal("explicit offer must win")
	}
}

func TestGuessTypeAndTitle(t *testing.T) {
	cases := map[string]string{
		"# Go 并发技能总结\n步骤...":              artifact.TypeSkill,
		"# 发布\n- [ ] a\n- [ ] b\n- [x] c": artifact.TypeChecklist,
		"# 学习笔记: Postgres\n...":           artifact.TypeNote,
		"# 架构说明\n正文":                      artifact.TypeDoc,
		"随便一段话":                           artifact.TypeOther,
	}
	for in, want := range cases {
		if got := artifact.GuessType(in); got != want {
			t.Errorf("GuessType(%q) = %s, want %s", in, got, want)
		}
	}
	if got := artifact.GuessTitle("前言\n# 真正标题\n正文"); got != "真正标题" {
		t.Fatalf("title = %q", got)
	}
}
