// internal/config/config.go
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr       string
	PostgresDSN    string
	RedisAddr      string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	OpenSearchURL  string
	QdrantURL      string
	VaultMasterKey string
	// CORSOrigins lists browser origins allowed to call the API (the web console).
	CORSOrigins []string
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid env line: %s", line)
		}
		vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	cfg := Config{
		HTTPAddr:       vals["HTTP_ADDR"],
		PostgresDSN:    vals["POSTGRES_DSN"],
		RedisAddr:      vals["REDIS_ADDR"],
		MinioEndpoint:  vals["MINIO_ENDPOINT"],
		MinioAccessKey: vals["MINIO_ACCESS_KEY"],
		MinioSecretKey: vals["MINIO_SECRET_KEY"],
		MinioBucket:    vals["MINIO_BUCKET"],
		OpenSearchURL:  vals["OPENSEARCH_URL"],
		QdrantURL:      vals["QDRANT_URL"],
		VaultMasterKey: vals["VAULT_MASTER_KEY"],
	}
	for _, o := range strings.Split(vals["CORS_ORIGINS"], ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}
	if cfg.PostgresDSN == "" || cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("missing required env")
	}
	return cfg, nil
}
