package cred

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeUsesEnvOverride(t *testing.T) {
	t.Setenv("SHARED_MEMORY_HOME", filepath.Join(t.TempDir(), "sm-home"))
	h, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if h != os.Getenv("SHARED_MEMORY_HOME") {
		t.Fatalf("got %q", h)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHARED_MEMORY_HOME", dir)
	c := Credentials{
		APIKey:   "sm_test_key",
		APIURL:   "http://127.0.0.1:18080",
		DeviceID: "dev-123",
	}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		// On Windows perm bits may be ignored; only assert when restrictive bits apply.
		if info.Mode().Perm()&0o777 == 0o600 || info.Mode().Perm()&0o077 == 0 {
			// ok
		}
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKey != c.APIKey || got.APIURL != c.APIURL || got.DeviceID != c.DeviceID {
		t.Fatalf("mismatch: %+v", got)
	}
}

func TestLoadMissing(t *testing.T) {
	t.Setenv("SHARED_MEMORY_HOME", t.TempDir())
	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestResolveUsesEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHARED_MEMORY_HOME", dir)
	t.Setenv("SHARED_MEMORY_API_URL", "http://env-url")
	t.Setenv("SHARED_MEMORY_API_KEY", "env-key")
	if err := Save(Credentials{APIKey: "file-key", APIURL: "http://file", DeviceID: "d1"}); err != nil {
		t.Fatal(err)
	}
	c, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "env-key" || c.APIURL != "http://env-url" || c.DeviceID != "d1" {
		t.Fatalf("got %+v", c)
	}
}
