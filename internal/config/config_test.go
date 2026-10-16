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

func TestLoad_EmbedderSettings(t *testing.T) {
	write := func(t *testing.T, extra string) string {
		p := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(p, []byte("HTTP_ADDR=:1\nPOSTGRES_DSN=postgres://x\n"+extra), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cfg, err := config.Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbedderURL != "" || cfg.EmbedderDim != 512 || cfg.VectorMinScore != 0 {
		t.Fatalf("defaults: %+v", cfg)
	}
	cfg, err = config.Load(write(t, "EMBEDDER_URL=http://e:80\nEMBEDDER_DIM=768\nVECTOR_MIN_SCORE=0.45\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbedderURL != "http://e:80" || cfg.EmbedderDim != 768 || cfg.VectorMinScore != 0.45 {
		t.Fatalf("parsed: %+v", cfg)
	}
	for _, bad := range []string{"EMBEDDER_DIM=0\n", "EMBEDDER_DIM=x\n", "VECTOR_MIN_SCORE=1.5\n"} {
		if _, err := config.Load(write(t, bad)); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

func TestLoad_EmbedderProtocolAndMaxDim(t *testing.T) {
	write := func(t *testing.T, extra string) string {
		p := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(p, []byte("HTTP_ADDR=:1\nPOSTGRES_DSN=postgres://x\n"+extra), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cfg, err := config.Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbedderProtocol != "tei" || cfg.EmbedderMaxDim != 4096 || cfg.EmbedderAPIKey != "" {
		t.Fatalf("defaults: %+v", cfg)
	}
	cfg, err = config.Load(write(t, "EMBEDDER_PROTOCOL=openai\nEMBEDDER_API_KEY=sk-test\nEMBEDDER_MAX_DIM=8192\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbedderProtocol != "openai" || cfg.EmbedderAPIKey != "sk-test" || cfg.EmbedderMaxDim != 8192 {
		t.Fatalf("parsed: %+v", cfg)
	}
	for _, bad := range []string{"EMBEDDER_PROTOCOL=ollama\n", "EMBEDDER_MAX_DIM=0\n", "EMBEDDER_MAX_DIM=-1\n"} {
		if _, err := config.Load(write(t, bad)); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
