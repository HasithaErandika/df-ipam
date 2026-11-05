# DF-IPAM developer commands. Run `make help` to list them.
# Settings come from api/.env and web/.env (see their .env.example files). PostgreSQL, the API and the worker
# run in Docker. Supabase provides sign-in, and its database is for testing only.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Sourcing in the shell, not `include`, keeps `$` and `#` in passwords intact.
LOAD_ENV := set -a; [ -f api/.env ] && . api/.env; set +a;
BIN := api/bin

COMPOSE := docker compose

.PHONY: help setup env-check dev api worker agent web \
        docker-build docker-up docker-down docker-restart docker-logs docker-ps docker-migrate docker-retention docker-rotate-secrets \
        db-up db-down db-shell db-reset db-backup test-db-supabase migrate-supabase \
        migrate-up migrate-down migrate-version seed db-setup retention rotate-secrets load-test \
        build build-api build-web test test-api test-db sql test-web lint lint-api lint-web vuln fmt clean

help: ## List available commands
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

setup: ## Install dependencies and create env files if missing
	@[ -f api/.env ] || { cp api/.env.example api/.env; echo "Created api/.env: fill in the values"; }
	@[ -f web/.env ] || { cp web/.env.example web/.env; echo "Created web/.env: fill in the values"; }
	cd api && go mod download
	cd web && pnpm install

env-check: ## Show which required settings are missing
	@$(LOAD_ENV) ok=1; for v in POSTGRES_PASSWORD DFIPAM_APP_PASSWORD DATABASE_URL MIGRATION_DATABASE_URL SUPABASE_URL SUPABASE_ANON_KEY SUPABASE_SERVICE_ROLE_KEY; do \
	  [ -n "$${!v}" ] || { echo "missing: $$v"; ok=0; }; done; \
	[ -f web/.env ] || { echo "missing: web/.env"; ok=0; }; \
	[ $$ok = 1 ] && echo "All required settings are present" || exit 1

dev: env-check docker-up web ## Run everything: Docker backend, then the web app

docker-build: ## Build the backend image
	@$(LOAD_ENV) $(COMPOSE) build

docker-up: ## Start PostgreSQL, migrate, seed, then start the API and worker
	@$(LOAD_ENV) $(COMPOSE) up -d --build --wait db api worker

docker-down: ## Stop the containers. Data stays in the volume
	@$(LOAD_ENV) $(COMPOSE) down

docker-restart: ## Restart the API and worker
	@$(LOAD_ENV) $(COMPOSE) restart api worker

docker-logs: ## Follow the API, worker and migration logs
	@$(LOAD_ENV) $(COMPOSE) logs -f api worker migrate

docker-ps: ## Show container status
	@$(LOAD_ENV) $(COMPOSE) ps -a

docker-migrate: ## Apply migrations and seed in the Docker database
	@$(LOAD_ENV) $(COMPOSE) run --rm migrate

docker-retention: ## Remove data older than its retention policy, in Docker
	@$(LOAD_ENV) $(COMPOSE) run --rm migrate /app/retention

docker-rotate-secrets: ## Re-encrypt integration credentials under the current key, in Docker
	@$(LOAD_ENV) $(COMPOSE) run --rm --no-deps api /app/rotate-secrets

db-up: ## Start only PostgreSQL
	@$(LOAD_ENV) $(COMPOSE) up -d --wait db

db-down: ## Stop only PostgreSQL. Data stays in the volume
	@$(LOAD_ENV) $(COMPOSE) stop db

db-shell: ## Open psql in the Docker database, as the owner
	@$(LOAD_ENV) $(COMPOSE) exec db psql -U postgres -d "$${POSTGRES_DB:-dfipam}"

db-reset: ## DELETE all local data and start empty. Needs CONFIRM=yes
	@[ "$(CONFIRM)" = "yes" ] || { echo "This deletes the local database. Run: make db-reset CONFIRM=yes"; exit 1; }
	@$(LOAD_ENV) $(COMPOSE) down -v

db-backup: ## Write an encrypted backup. Needs BACKUP_GPG_RECIPIENT
	@$(LOAD_ENV) deploy/scripts/backup.sh

api: ## Run the API server
	@$(LOAD_ENV) cd api && go run ./cmd/api

worker: ## Run the background worker
	@$(LOAD_ENV) cd api && go run ./cmd/worker

agent: ## Run the discovery agent
	@$(LOAD_ENV) cd api && go run ./cmd/agent

web: ## Run the web app dev server
	cd web && pnpm dev

migrate-up: ## Apply migrations from this machine to DATABASE_URL's server
	@$(LOAD_ENV) cd api && go run ./cmd/migrate up

migrate-down: ## Roll back the latest migration
	@$(LOAD_ENV) cd api && go run ./cmd/migrate down

migrate-version: ## Show the current migration version
	@$(LOAD_ENV) cd api && go run ./cmd/migrate version

seed: ## Load roles, permissions and default settings (safe to repeat)
	@$(LOAD_ENV) cd api && go run ./cmd/seed

db-setup: migrate-up seed ## Migrate, then seed

retention: ## Remove data older than its retention policy (owner role)
	@$(LOAD_ENV) cd api && go run ./cmd/retention

rotate-secrets: ## Re-encrypt integration credentials under the current key
	@$(LOAD_ENV) cd api && go run ./cmd/rotate-secrets

load-test: ## Measure read latency. Needs LOADTEST_TOKEN; use a test system
	@$(LOAD_ENV) cd api && go run ./cmd/loadtest

build: build-api build-web ## Build everything

build-api: ## Build the Go binaries into api/bin
	cd api && go build -o bin/ ./cmd/...

build-web: ## Build the web app into web/dist
	cd web && pnpm build

test: test-api test-web ## Run all tests

test-api: ## Run Go tests
	cd api && go test ./...

test-db: ## Run database tests on local PostgreSQL. Needs `make db-up` and migrations
	@$(LOAD_ENV) cd api && TEST_DATABASE_URL="$$MIGRATION_DATABASE_URL" TEST_APP_DATABASE_URL="$$DATABASE_URL" TEST_DB_MAX_CONNS=10 go test -p 1 -count=1 -v ./internal/dbrules ./internal/server ./internal/modules/allocation

test-db-supabase: ## Run database tests on Supabase: rule and API tests roll back; concurrency tests delete what they create
	@$(LOAD_ENV) cd api && TEST_DATABASE_URL="$$SUPABASE_MIGRATION_DATABASE_URL" TEST_APP_DATABASE_URL="$$SUPABASE_DATABASE_URL" TEST_DB_MAX_CONNS=10 go test -p 1 -count=1 -v ./internal/dbrules ./internal/server ./internal/modules/allocation

migrate-supabase: ## Apply migrations and seed to the Supabase test database
	@$(LOAD_ENV) cd api && MIGRATION_DATABASE_URL="$$SUPABASE_MIGRATION_DATABASE_URL" go run ./cmd/migrate up && MIGRATION_DATABASE_URL="$$SUPABASE_MIGRATION_DATABASE_URL" go run ./cmd/seed

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
