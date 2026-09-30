.PHONY: test up down api
test:
	go test ./...
up:
	docker compose up -d
down:
	docker compose down
api:
	SHARED_MEMORY_ENV=.env go run ./cmd/api
