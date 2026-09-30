// cmd/api/main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"sharedmemory/internal/config"
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
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	fmt.Println("listening", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, mux))
}
