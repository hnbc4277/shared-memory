// internal/config/config.go
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr       string
	PostgresDSN    string
	RedisAddr      string
	// JobStream and JobGroup override the Redis Streams queue (empty = sm:jobs / sm-workers).
	JobStream string
	JobGroup  string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	OpenSearchURL  string
	QdrantURL      string
	VaultMasterKey string
	// CORSOrigins lists browser origins allowed to call the API (the web console).
	CORSOrigins []string
	// EmbedderURL points at a text-embeddings-inference server; empty selects
	// the offline hash embedder.
	EmbedderURL   string
	EmbedderModel string
	EmbedderDim   int
	// VectorMinScore overrides the embedder's default cosine cut-off when > 0.
	VectorMinScore float64
	// OCRLangs are tesseract language packs for image attachments; empty disables OCR.
	OCRLangs string
	// LLMDeriverURL is an OpenAI-compatible chat completions base (…/v1); empty keeps Heuristic.
	LLMDeriverURL    string
	LLMDeriverAPIKey string
	LLMDeriverModel  string
	// VisionURL is an OpenAI-compatible multimodal chat completions base; empty keeps OCR/metadata only.
	VisionURL    string
	VisionAPIKey string
	VisionModel  string
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
		EmbedderURL:    vals["EMBEDDER_URL"],
		EmbedderModel:  "bge-small-zh-v1.5",
		EmbedderDim:    512,
		OCRLangs:         vals["OCR_LANGS"],
		LLMDeriverURL:    vals["LLM_DERIVER_URL"],
		LLMDeriverAPIKey: vals["LLM_DERIVER_API_KEY"],
		LLMDeriverModel:  firstNonEmpty(vals["LLM_DERIVER_MODEL"], "gpt-4o-mini"),
		VisionURL:        vals["VISION_URL"],
		VisionAPIKey:     vals["VISION_API_KEY"],
		VisionModel:      firstNonEmpty(vals["VISION_MODEL"], "gpt-4o-mini"),
	}
	if v := vals["EMBEDDER_MODEL"]; v != "" {
		cfg.EmbedderModel = v
	}
	if v := vals["EMBEDDER_DIM"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid EMBEDDER_DIM: %q", v)
		}
		cfg.EmbedderDim = n
	}
	if v := vals["VECTOR_MIN_SCORE"]; v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f >= 1 {
			return Config{}, fmt.Errorf("invalid VECTOR_MIN_SCORE: %q", v)
		}
		cfg.VectorMinScore = f
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
