.PHONY: test integration e2e smoke up down app api worker mcp web
test:
	go test ./...
integration:
	go test -tags=integration ./...
e2e:
	go test -tags=e2e ./internal/e2e/ -v
smoke:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/e2e-smoke.ps1
up:
	docker compose up -d
down:
	docker compose --profile app down
app:
	docker compose --profile app up -d --build
api:
	SHARED_MEMORY_ENV=.env go run ./cmd/api
worker:
	SHARED_MEMORY_ENV=.env go run ./cmd/worker
mcp:
	go build -o bin/shared-memory-mcp ./cmd/mcp
web:
	cd web && npm run dev
