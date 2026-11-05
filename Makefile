# DF-IPAM developer commands. Run `make help` to list them.
# Settings come from api/.env and web/.env (see their .env.example files). Supabase provides the database and
# sign-in, so nothing runs locally except the API and the web app.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Sourcing in the shell, not `include`, keeps `$` and `#` in passwords intact.
LOAD_ENV := set -a; [ -f api/.env ] && . api/.env; set +a;
BIN := api/bin

.PHONY: help setup env-check dev api worker agent web \
        migrate-up migrate-down migrate-version seed db-setup \
        build build-api build-web test test-api test-db sql test-web lint lint-api lint-web vuln fmt clean

help: ## List available commands
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

setup: ## Install dependencies and create env files if missing
	@[ -f api/.env ] || { cp api/.env.example api/.env; echo "Created api/.env: fill in the values"; }
	@[ -f web/.env ] || { cp web/.env.example web/.env; echo "Created web/.env: fill in the values"; }
	cd api && go mod download
	cd web && pnpm install

env-check: ## Show which required settings are missing
	@$(LOAD_ENV) ok=1; for v in DATABASE_URL MIGRATION_DATABASE_URL SUPABASE_URL SUPABASE_ANON_KEY SUPABASE_SERVICE_ROLE_KEY; do \
	  [ -n "$${!v}" ] || { echo "missing: $$v"; ok=0; }; done; \
	[ -f web/.env ] || { echo "missing: web/.env"; ok=0; }; \
	[ $$ok = 1 ] && echo "All required settings are present" || exit 1

dev: env-check ## Run the API and the web app together
	@$(MAKE) -j2 api web

api: ## Run the API server
	@$(LOAD_ENV) cd api && go run ./cmd/api

worker: ## Run the background worker
	@$(LOAD_ENV) cd api && go run ./cmd/worker

agent: ## Run the discovery agent
	@$(LOAD_ENV) cd api && go run ./cmd/agent

web: ## Run the web app dev server
	cd web && pnpm dev

migrate-up: ## Apply all database migrations
	@$(LOAD_ENV) cd api && go run ./cmd/migrate up

migrate-down: ## Roll back the latest migration
	@$(LOAD_ENV) cd api && go run ./cmd/migrate down

migrate-version: ## Show the current migration version
	@$(LOAD_ENV) cd api && go run ./cmd/migrate version

seed: ## Load roles, permissions and default settings (safe to repeat)
	@$(LOAD_ENV) cd api && go run ./cmd/seed

db-setup: migrate-up seed ## Migrate, then seed

build: build-api build-web ## Build everything

build-api: ## Build the Go binaries into api/bin
	cd api && go build -o bin/ ./cmd/...

build-web: ## Build the web app into web/dist
	cd web && pnpm build

test: test-api test-web ## Run all tests

test-api: ## Run Go tests
	cd api && go test ./...

test-db: ## Run database tests: rule and API tests roll back; concurrency tests delete what they create
	@$(LOAD_ENV) cd api && TEST_DATABASE_URL="$$MIGRATION_DATABASE_URL" TEST_APP_DATABASE_URL="$$DATABASE_URL" go test -count=1 -v ./internal/dbrules ./internal/server ./internal/modules/allocation

sql: ## Regenerate api/sql/supabase-setup.sql from the migrations and seed
	cd api && go run ./cmd/sqlbundle > sql/supabase-setup.sql

test-web: ## Run web tests
	cd web && pnpm test

lint: lint-api lint-web ## Run all linters

lint-api: ## Check Go formatting and run go vet
	@cd api && test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	cd api && go vet ./...

lint-web: ## Type-check the web app
	cd web && pnpm lint

vuln: ## Scan dependencies for known vulnerabilities
	cd api && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	cd web && pnpm audit

fmt: ## Format Go code
	cd api && gofmt -w .

clean: ## Remove build output
	rm -rf api/bin web/dist
