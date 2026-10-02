// internal/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"sharedmemory/internal/config"
)

func TestLoad_ReadsProjectEnvOnly(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "" +
		"HTTP_ADDR=:18080\n" +
		"POSTGRES_DSN=postgres://sm:sm@127.0.0.1:15432/sharedmemory?sslmode=disable\n" +
		"REDIS_ADDR=127.0.0.1:16379\n" +
		"MINIO_ENDPOINT=127.0.0.1:19000\n" +
		"MINIO_ACCESS_KEY=smminio\n" +
		"MINIO_SECRET_KEY=smminio_secret\n" +
		"MINIO_BUCKET=shared-memory\n" +
		"OPENSEARCH_URL=http://127.0.0.1:19200\n" +
		"QDRANT_URL=http://127.0.0.1:16333\n" +
		"VAULT_MASTER_KEY=0123456789abcdef0123456789abcdef\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PostgresDSN == "" || cfg.HTTPAddr != ":18080" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.PostgresDSN != "postgres://sm:sm@127.0.0.1:15432/sharedmemory?sslmode=disable" {
		t.Fatalf("must use project ports, got %s", cfg.PostgresDSN)
	}
}

func TestLoad_ParsesCORSOrigins(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	content := "HTTP_ADDR=:18080\nPOSTGRES_DSN=postgres://x\nCORS_ORIGINS= http://a:1 , http://b:2 ,\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "http://a:1" || cfg.CORSOrigins[1] != "http://b:2" {
		t.Fatalf("unexpected origins: %#v", cfg.CORSOrigins)
	}
}
