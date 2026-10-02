package vault_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/vault"
)

func TestDetectAndRedact(t *testing.T) {
	msg := "公司 GitLab https://gitlab.example.com/login 账号: sxc 密码: S3cr3t!x 记一下"
	found := vault.Detect(msg)
	if len(found) != 1 {
		t.Fatalf("want 1 candidate, got %+v", found)
	}
	d := found[0]
	if d.Secret != "S3cr3t!x" || d.Username != "sxc" || d.SiteURL != "https://gitlab.example.com/login" || d.Title != "gitlab.example.com" {
		t.Fatalf("unexpected detection %+v", d)
	}
	red := vault.Redact(msg)
	if strings.Contains(red, "S3cr3t!x") || !strings.Contains(red, vault.Redacted) {
		t.Fatalf("redaction failed: %q", red)
	}
	if !strings.Contains(red, "账号: sxc") {
		t.Fatalf("redaction must keep non-secret text: %q", red)
	}
}

func TestDetect_EnglishTokenAndNoFalsePositive(t *testing.T) {
	if got := vault.Detect("my api_key=abc123def please"); len(got) != 1 || got[0].Secret != "abc123def" {
		t.Fatalf("api key detection: %+v", got)
	}
	if got := vault.Detect("we discussed password policies today"); len(got) != 0 {
		t.Fatalf("false positive: %+v", got)
	}
}

func TestCipher_RoundTripAndAccountIsolation(t *testing.T) {
	c, err := vault.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	a, b := uuid.New(), uuid.New()
	ct, nonce, err := c.Encrypt(a, "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("hunter2")) {
		t.Fatal("ciphertext contains plaintext")
	}
	pt, err := c.Decrypt(a, ct, nonce)
	if err != nil || pt != "hunter2" {
		t.Fatalf("decrypt: %v %q", err, pt)
	}
	if _, err := c.Decrypt(b, ct, nonce); err == nil {
		t.Fatal("other account key must not decrypt")
	}
	if _, err := vault.NewCipher("short"); err == nil {
		t.Fatal("short master key must be rejected")
	}
}
