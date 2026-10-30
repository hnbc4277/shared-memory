package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"sharedmemory/internal/account"
	"sharedmemory/internal/agents"
	"sharedmemory/internal/artifact"
	"sharedmemory/internal/attachment"
	"sharedmemory/internal/audit"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/config"
	"sharedmemory/internal/db"
	"sharedmemory/internal/device"
	"sharedmemory/internal/entity"
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/index"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/quarantine"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/ratelimit"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/reindex"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/session"
	"sharedmemory/internal/vault"
	"sharedmemory/internal/worker"
)

// App holds every service built from one Config; API, worker and e2e tests share it.
type App struct {
	Config    config.Config
	Pool      *pgxpool.Pool
	Redis     *redis.Client
	Objects   objectstore.Store
	Streams   *queue.Streams
	Index     *index.Service
	Memories  *memory.Store
	Deps      httpapi.Deps
	Worker    *worker.Handler
	Reindexer *reindex.Reindexer
}

// FindMigrations locates the migrations directory from the repo root or a sub-package.
func FindMigrations() string {
	for _, p := range []string{"migrations", filepath.Join("..", "migrations"), filepath.Join("..", "..", "migrations")} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "migrations"
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	pool, err := db.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		return nil, err
	}
	a := &App{Config: cfg, Pool: pool}
	fail := func(err error) (*App, error) {
		a.Close()
		return nil, err
	}
	if err := db.Migrate(ctx, pool, FindMigrations()); err != nil {
		return fail(fmt.Errorf("migrate: %w", err))
	}
	objects, err := objectstore.NewMinIO(ctx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, false)
	if err != nil {
		return fail(err)
	}
	a.Objects = objects
	a.Redis = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	a.Streams = queue.NewStreams(a.Redis, cfg.JobStream, cfg.JobGroup)
	if err := a.Streams.EnsureGroup(ctx); err != nil {
		log.Printf("redis unavailable, ingest will mark sessions degraded: %v", err)
	}
	cipher, err := vault.NewCipher(cfg.VaultMasterKey)
	if err != nil {
		return fail(err)
	}

	var embedder semantic.Embedder = semantic.HashEmbedder{}
	collection, minScore := index.DefaultCollection, index.DefaultMinScore
	if cfg.EmbedderURL != "" {
		embedder = semantic.NewTEIEmbedder(cfg.EmbedderURL, cfg.EmbedderDim)
		collection, minScore = index.CollectionFor(cfg.EmbedderModel, cfg.EmbedderDim), index.DefaultModelMinScore
	}
	if cfg.VectorMinScore > 0 {
		minScore = cfg.VectorMinScore
	}
	vector := index.NewQdrant(cfg.QdrantURL, collection, embedder.Dim())
	vector.MinScore = minScore
	a.Index = &index.Service{
		Text:     index.NewOpenSearch(cfg.OpenSearchURL, ""),
		Vector:   vector,
		Embedder: embedder,
	}
	if err := a.Index.Ensure(ctx); err != nil {
		log.Printf("search index unavailable, recall falls back to postgres: %v", err)
	}
	log.Printf("vector collection %s (dim %d, min score %.2f)", collection, embedder.Dim(), minScore)
	embStore := semantic.NewStore(pool, cipher)
	embResolver := &semantic.Resolver{
		Settings:       embStore,
		DeployProtocol: cfg.EmbedderProtocol,
		DeployURL:      cfg.EmbedderURL,
		DeployModel:    cfg.EmbedderModel,
		DeployAPIKey:   cfg.EmbedderAPIKey,
		DeployDim:      cfg.EmbedderDim,
		MaxDim:         cfg.EmbedderMaxDim,
	}
	a.Index.Resolver = embResolver
	a.Memories = memory.NewStore(pool, a.Index)

	authStore := auth.NewStore(pool)
	projects := project.NewStore(pool)
	policies := policy.NewStore(pool)
	vaults := vault.NewStore(pool, cipher)
	artifacts := artifact.NewStore(pool, objects)
	attachments := &attachment.Service{Pool: pool, Objects: objects, Policies: policies, Projects: projects, Queue: a.Streams}
	kbStore := kb.NewStore(pool, objects, a.Streams, a.Index).WithSources(artifacts, attachments)
	entities := &entity.Store{Pool: pool}
	a.Deps = httpapi.Deps{
		Pool:     pool,
		Auth:     authStore,
		Projects: projects,
		Policies: policies,
		Sessions: &session.Service{Pool: pool, Objects: objects, Queue: a.Streams, Policies: policies,
			Projects: projects, Vault: vaults, Artifacts: artifacts},
		Attachments:   attachments,
		Memories:      a.Memories,
		Recall:        &recall.Service{Pool: pool, Index: a.Index, Memories: a.Memories, Policies: policies, Entities: entities, KB: kbStore},
		Vault:         vaults,
		Artifacts:     artifacts,
		Audit:         audit.NewLog(pool),
		Deleter:       &account.Deleter{Pool: pool, Objects: objects, Index: a.Index},
		Index:         a.Index,
		Devices:       device.NewStore(pool, authStore),
		Quarantine:    &quarantine.Store{Pool: pool},
		Entities:      entities,
		KB:            kbStore,
		Agents:        agents.NewStore(pool, authStore),
		Embedding:     embStore,
		EmbedResolver: embResolver,
		EmbedMaxDim:   cfg.EmbedderMaxDim,
		RateLimit: &ratelimit.Limiter{
			Redis: a.Redis, Limit: cfg.RateLimitPerMin, PublicLimit: cfg.AuthRateLimitPerMin,
			EmailAuthLimit: cfg.AuthEmailRateLimitPerMin, Window: time.Minute,
		},
		CORSOrigins:  cfg.CORSOrigins,
		WebStaticDir: cfg.WebStaticDir,
		DownloadsDir: resolveDownloadsDir(),
		PublicAPIURL: cfg.PublicAPIURL,
	}
	if strings.TrimSpace(a.Deps.PublicAPIURL) != "" {
		log.Printf("PUBLIC_API_URL=%s", a.Deps.PublicAPIURL)
	} else {
		log.Printf("PUBLIC_API_URL unset (setup guides will fall back to console API base)")
	}
	if a.Deps.DownloadsDir != "" {
		log.Printf("client downloads from %s", a.Deps.DownloadsDir)
	}
	deriver := semantic.Deriver(semantic.Heuristic{})
	if cfg.LLMDeriverURL != "" {
		deriver = semantic.NewLLMDeriver(cfg.LLMDeriverURL, cfg.LLMDeriverAPIKey, cfg.LLMDeriverModel)
		log.Printf("llm deriver enabled (%s)", cfg.LLMDeriverModel)
	}
	extractor := worker.NewExtractor(cfg.OCRLangs)
	if cfg.VisionURL != "" {
		extractor.Vision = worker.NewVisionClient(cfg.VisionURL, cfg.VisionAPIKey, cfg.VisionModel)
		log.Printf("vision describer enabled (%s)", cfg.VisionModel)
	}
	a.Worker = &worker.Handler{Pool: pool, Objects: objects, Memories: a.Memories,
		Deriver: deriver, Extractor: extractor, Entities: entities, Policies: policies, KB: kbStore}
	a.Reindexer = &reindex.Reindexer{Pool: pool, Index: a.Index, Memories: a.Memories, Artifacts: artifacts,
		Collection: collection, Resolver: embResolver, Settings: embStore}
	a.Deps.Reindexer = a.Reindexer
	return a, nil
}

func (a *App) Handler() http.Handler {
	return httpapi.NewRouter(a.Deps)
}

func (a *App) Close() {
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	if a.Pool != nil {
		a.Pool.Close()
	}
}

func resolveDownloadsDir() string {
	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "web", "public", "downloads"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "downloads"),
			filepath.Join(dir, "..", "web", "public", "downloads"),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return ""
}
