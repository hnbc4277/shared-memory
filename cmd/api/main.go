package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"sharedmemory/internal/config"
	"sharedmemory/internal/db"
	"sharedmemory/internal/httpapi"
)

func main() {
	path := ".env"
	if v := os.Getenv("SHARED_MEMORY_ENV"); v != "" {
		path = v
	}
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	migrations := "migrations"
	if _, err := os.Stat(migrations); err != nil {
		migrations = filepath.Join("..", "..", "migrations")
	}
	if err := db.Migrate(ctx, pool, migrations); err != nil {
		log.Fatal(err)
	}

	handler := httpapi.NewRouter(pool)
	log.Println("listening", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, handler))
}
