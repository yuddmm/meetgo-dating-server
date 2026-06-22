.PHONY: help run dev build tidy vet test sqlc swag \
        migrate-up migrate-down migrate-create \
        docker-up docker-down docker-logs

# Load .env if present so DATABASE_URL etc. are available to targets.
ifneq (,$(wildcard .env))
include .env
export
endif

COMPOSE := docker compose -f deployments/docker-compose.yml
MIGRATIONS_DIR := migrations

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

run: ## Run the server locally
	go run ./cmd/server

dev: ## Run with hot-reload (requires air: go install github.com/air-verse/air@latest)
	air

build: ## Build the server binary into ./bin
	go build -o bin/server ./cmd/server

tidy: ## Tidy go.mod / go.sum
	go mod tidy

vet: ## Run go vet
	go vet ./...

test: ## Run tests
	go test ./...

sqlc: ## Generate type-safe code from SQL (requires sqlc)
	sqlc generate

swag: ## Generate Swagger/OpenAPI docs into internal/docs (requires swag)
	swag init -g cmd/server/main.go -o internal/docs --parseInternal

migrate-up: ## Apply all up migrations (requires migrate CLI; uses DATABASE_URL)
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

migrate-create: ## Create a new migration: make migrate-create name=add_users
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

docker-up: ## Start postgres + app via docker compose
	$(COMPOSE) up -d --build

docker-down: ## Stop and remove docker compose stack
	$(COMPOSE) down

docker-logs: ## Tail app logs
	$(COMPOSE) logs -f app
