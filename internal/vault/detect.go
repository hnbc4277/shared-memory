package vault

import (
	"net/url"
	"regexp"
	"strings"
)

const Redacted = "[REDACTED]"

var (
	secretRe   = regexp.MustCompile(`(?i)(password|passwd|pwd|passcode|密码|口令|token|api[_-]?key|secret)\s*(?:[:：=]|是|为)\s*["']?([^\s"'，。,;；]+)`)
	usernameRe = regexp.MustCompile(`(?i)(username|user|account|login|账号|帐号|用户名)\s*(?:[:：=]|是|为)\s*["']?([^\s"'，。,;；]+)`)
	urlRe      = regexp.MustCompile(`https?://[^\s"'<>，。]+`)
)

// Detected is one credential candidate found in a message.
type Detected struct {
	Title    string
	SiteURL  string
	Username string
	Secret   string
}

// Detect finds credential-like secrets. The nearest URL and username in the
// same message are attached to every secret found.
func Detect(content string) []Detected {
	matches := secretRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	site := urlRe.FindString(content)
	user := ""
	if m := usernameRe.FindStringSubmatch(content); m != nil {
		user = m[2]
	}
	title := site
	if u, err := url.Parse(site); err == nil && u.Host != "" {
		title = u.Host
	}
	var out []Detected
	seen := map[string]bool{}
	for _, m := range matches {
		secret := m[2]
		if secret == Redacted || seen[secret] {
			continue
		}
		seen[secret] = true
		t := title
		if t == "" {
			t = strings.ToLower(m[1])
		}
		out = append(out, Detected{Title: t, SiteURL: site, Username: user, Secret: secret})
	}
	return out
}

// Redact replaces secret values so they never reach transcripts, summaries or indexes.
func Redact(content string) string {
	return secretRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := secretRe.FindStringSubmatch(match)
		if len(sub) < 3 || sub[2] == "" {
			return match
		}
		idx := strings.LastIndex(match, sub[2])
		return match[:idx] + Redacted + match[idx+len(sub[2]):]
	})
}

func ContainsSecret(content string) bool {
	return len(Detect(content)) > 0
}
