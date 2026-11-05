.DEFAULT_GOAL := help
SHELL := /bin/bash

COMPOSE := docker compose -f infrastructure/docker/docker-compose.yml
API_DIR := apps/api

.PHONY: help setup db-up db-down db-reset storage-setup migrate-up migrate-down migrate-status api api-build api-test api-test-integration api-vet contracts contracts-check web web-build web-lint web-typecheck check

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

setup: ## Install dependencies and create local env files if missing
	pnpm install
	cd $(API_DIR) && go mod download
	@test -f $(API_DIR)/.env || cp $(API_DIR)/.env.example $(API_DIR)/.env
	@test -f apps/web/.env.local || cp apps/web/.env.example apps/web/.env.local

db-up: ## Start local Postgres, object storage and NATS and wait until healthy
	$(COMPOSE) up -d --wait postgres storage nats

db-down: ## Stop local services (keeps data)
	$(COMPOSE) down

db-reset: ## Stop local services and delete their data
	$(COMPOSE) down -v

storage-setup: ## Create the media bucket and allow browser uploads (needs db-up)
	cd $(API_DIR) && set -a && source .env && set +a && go run ./cmd/storage-setup

migrate-up: ## Apply pending database migrations
	cd $(API_DIR) && set -a && source .env && set +a && go run ./cmd/migrate up

migrate-down: ## Roll back the most recent migration group
	cd $(API_DIR) && set -a && source .env && set +a && go run ./cmd/migrate down

migrate-status: ## Show applied and pending migrations
	cd $(API_DIR) && set -a && source .env && set +a && go run ./cmd/migrate status

api: ## Run the API with apps/api/.env
	cd $(API_DIR) && set -a && source .env && set +a && go run ./cmd/api

api-build: ## Build the API binary into apps/api/bin
	cd $(API_DIR) && go build -o bin/api ./cmd/api

api-test: ## Run API tests
	cd $(API_DIR) && go test ./...

api-test-integration: ## Run API tests including Postgres, storage and NATS integration tests (needs db-up, migrate-up, storage-setup)
	cd $(API_DIR) && set -a && source .env && set +a && CAVI_TEST_DATABASE_URL="$$CAVI_DATABASE_URL" CAVI_TEST_STORAGE_ENDPOINT="$$CAVI_STORAGE_ENDPOINT" CAVI_TEST_NATS_URL="$$CAVI_NATS_URL" go test -count=1 ./...

api-vet: ## Run go vet on the API
	cd $(API_DIR) && go vet ./...

contracts: ## Regenerate the TypeScript API client from the OpenAPI spec
	pnpm --filter @cavi/api-client generate

contracts-check: ## Fail if the generated API client is out of date
	pnpm --filter @cavi/api-client check:generated

web: ## Run the web app in development mode
	pnpm --filter @cavi/web dev

web-build: ## Production build of the web app
	pnpm --filter @cavi/web build

web-lint: ## Lint the web app
	pnpm --filter @cavi/web lint

web-typecheck: ## Type-check the web app and API client
	pnpm --filter @cavi/api-client typecheck
	pnpm --filter @cavi/web typecheck

check: api-vet api-test contracts-check web-lint web-typecheck web-build ## Run all static checks, tests and the production web build
