.DEFAULT_GOAL := help
SHELL := /bin/bash

COMPOSE := docker compose -f infrastructure/docker/docker-compose.yml
API_DIR := apps/api

.PHONY: help setup db-up db-down db-reset api api-build api-test api-vet web web-build web-lint web-typecheck check

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

setup: ## Install dependencies and create local env files if missing
	pnpm install
	cd $(API_DIR) && go mod download
	@test -f $(API_DIR)/.env || cp $(API_DIR)/.env.example $(API_DIR)/.env
	@test -f apps/web/.env.local || cp apps/web/.env.example apps/web/.env.local

db-up: ## Start local Postgres and wait until healthy
	$(COMPOSE) up -d --wait postgres

db-down: ## Stop local Postgres (keeps data)
	$(COMPOSE) down

db-reset: ## Stop local Postgres and delete its data
	$(COMPOSE) down -v

api: ## Run the API with apps/api/.env
	cd $(API_DIR) && set -a && source .env && set +a && go run ./cmd/api

api-build: ## Build the API binary into apps/api/bin
	cd $(API_DIR) && go build -o bin/api ./cmd/api

api-test: ## Run API tests
	cd $(API_DIR) && go test ./...

api-vet: ## Run go vet on the API
	cd $(API_DIR) && go vet ./...

web: ## Run the web app in development mode
	pnpm --filter @cavi/web dev

web-build: ## Production build of the web app
	pnpm --filter @cavi/web build

web-lint: ## Lint the web app
	pnpm --filter @cavi/web lint

web-typecheck: ## Type-check the web app
	pnpm --filter @cavi/web typecheck

check: api-vet api-test web-lint web-typecheck ## Run all static checks and tests
