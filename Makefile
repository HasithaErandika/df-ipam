# DF-IPAM developer commands. Run `make help` to list them.
# Settings come from api/.env and web/.env (see their .env.example files). PostgreSQL, the backend and the web
# app run in Docker. People sign in with Microsoft Entra ID.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Sourcing in the shell, not `include`, keeps `$` and `#` in passwords intact.
LOAD_ENV := set -a; APP_COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || true); [ -f api/.env ] && . api/.env; [ -f web/.env ] && . web/.env; COMPOSE_FILE=$${COMPOSE_FILE:-docker-compose.yml:docker-compose.dev.yml$$([ "$${HOST_NETWORK:-}" = true ] && echo :docker-compose.hostnet.yml)}; set +a;
BIN := api/bin

COMPOSE := docker compose

.PHONY: help setup env-check dev backend web apache-up apache-web \
        docker-build docker-up docker-down docker-restart docker-logs docker-ps docker-migrate \
        db-up db-down db-shell db-reset db-backup admin \
        migrate-up migrate-down migrate-version seed db-setup lint-size \
        build build-api build-web test test-api test-db test-web lint lint-api lint-web vuln fmt clean

help: ## List available commands
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

setup: ## Install dependencies and create env files if missing
	@[ -f api/.env ] || { cp api/.env.example api/.env; echo "Created api/.env: fill in the values"; }
	@[ -f web/.env ] || { cp web/.env.example web/.env; echo "Created web/.env: fill in the values"; }
	cd api && go mod download
	cd web && pnpm install

env-check: ## Show which required settings are missing
	@$(LOAD_ENV) ok=1; for v in POSTGRES_PASSWORD DFIPAM_APP_PASSWORD DATABASE_URL MIGRATION_DATABASE_URL; do \
	  [ -n "$${!v}" ] || { echo "missing: $$v"; ok=0; }; done; \
	[ -f web/.env ] || { echo "missing: web/.env"; ok=0; }; \
	[ $$ok = 1 ] && echo "All required settings are present" || exit 1

dev: env-check docker-up web ## Run everything: Docker backend, then the web app

docker-build: ## Build the backend image
	@$(LOAD_ENV) $(COMPOSE) build

docker-up: ## Start PostgreSQL, migrate, seed, then start the backend and web app
	@$(LOAD_ENV) $(COMPOSE) up -d --build --wait --remove-orphans db backend web

apache-up: ## Start PostgreSQL and the backend for Apache (no web container): API on 127.0.0.1:8080
	@$(LOAD_ENV) COMPOSE_FILE=docker-compose.yml:docker-compose.apache.yml$$([ "$${HOST_NETWORK:-}" = true ] && echo :docker-compose.hostnet.yml) \
		$(COMPOSE) up -d --build --wait --remove-orphans db backend

apache-web: ## Build the web app for Apache into web/dist (copy it to /var/www/df-ipam)
	@$(LOAD_ENV) cd web && VITE_API_URL= pnpm build && echo "Built web/dist. Copy it with: sudo rsync -a --delete web/dist/ /var/www/df-ipam/"

docker-down: ## Stop the containers. Data stays in the volume
	@$(LOAD_ENV) $(COMPOSE) down

docker-restart: ## Restart the backend and web app
	@$(LOAD_ENV) $(COMPOSE) restart backend web

docker-logs: ## Follow the backend, web and migration logs
	@$(LOAD_ENV) $(COMPOSE) logs -f backend web migrate

docker-ps: ## Show container status
	@$(LOAD_ENV) $(COMPOSE) ps -a

docker-migrate: ## Apply migrations and seed in the Docker database
	@$(LOAD_ENV) $(COMPOSE) run --rm migrate



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

backend: ## Run the API without Docker
	@$(LOAD_ENV) cd api && go run ./cmd/server





web: ## Run the web app dev server
	cd web && pnpm dev

migrate-up: ## Apply migrations from this machine to DATABASE_URL's server
	@$(LOAD_ENV) cd api && go run ./cmd/migrate up

migrate-down: ## Roll back the latest migration
	@$(LOAD_ENV) cd api && go run ./cmd/migrate down

migrate-version: ## Show the current migration version
	@$(LOAD_ENV) cd api && go run ./cmd/migrate version

seed: ## Load default environments and settings (safe to repeat)
	@$(LOAD_ENV) cd api && go run ./cmd/seed

db-setup: migrate-up seed ## Migrate, then seed

admin: ## Make someone an Administrator by work email: make admin EMAIL=name@dialogfinance.lk
	@$(LOAD_ENV) cd api && go run ./cmd/admin -email "$(EMAIL)"




build: build-api build-web ## Build everything

build-api: ## Build the Go binaries into api/bin
	cd api && go build -o bin/ ./cmd/...

build-web: ## Build the web app into web/dist
	cd web && pnpm build

test: test-api test-web ## Run all tests

test-api: ## Run Go tests
	cd api && go test ./...

test-db: ## Run database tests on a throwaway dfipam_test database (dropped afterwards). Needs `make db-up`
	@$(LOAD_ENV) $(COMPOSE) exec -T db psql -q -U postgres -c 'DROP DATABASE IF EXISTS dfipam_test' -c 'CREATE DATABASE dfipam_test'
	@$(LOAD_ENV) db="$${POSTGRES_DB:-dfipam}"; owner="$${MIGRATION_DATABASE_URL/\/$$db\?/\/dfipam_test?}"; app="$${DATABASE_URL/\/$$db\?/\/dfipam_test?}"; \
	cd api && MIGRATION_DATABASE_URL="$$owner" go run ./cmd/migrate up && MIGRATION_DATABASE_URL="$$owner" go run ./cmd/seed && \
	TEST_DATABASE_URL="$$owner" TEST_APP_DATABASE_URL="$$app" TEST_DB_MAX_CONNS=10 go test -p 1 -count=1 -v ./internal/dbrules ./internal/server ./internal/scanner; \
	status=$$?; cd .. && $(COMPOSE) exec -T db psql -q -U postgres -c 'DROP DATABASE IF EXISTS dfipam_test'; exit $$status

test-web: ## Run web tests
	cd web && pnpm test

lint: lint-api lint-web ## Run all linters

lint-api: ## Check Go formatting, run go vet, and the size and complexity rules
	@cd api && test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	cd api && go vet ./...
	@$(MAKE) --no-print-directory lint-size

# CLAUDE.md: files under 200 lines, functions at cognitive complexity 15 or less.
# Tests are left out: a table of cases reads better whole.
lint-size: ## Check the 200-line file and complexity-15 function rules
	@long="$$(find api web/src \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \) ! -name '*_test.go' ! -name '*.test.ts' ! -name '*.test.tsx' \
		! -path '*/node_modules/*' -exec awk 'END { if (NR > 200) print NR, FILENAME }' {} \;)"; \
		if [ -n "$$long" ]; then echo "Files over 200 lines:"; echo "$$long"; exit 1; fi
	@complex="$$(cd api && go run github.com/uudashr/gocognit/cmd/gocognit@v1.2.0 -over 15 -ignore '_test.go' . 2>&1 || true)"; \
		if [ -n "$$complex" ]; then echo "Functions over cognitive complexity 15:"; echo "$$complex"; exit 1; fi

lint-web: ## Type-check the web app
	cd web && pnpm lint

vuln: ## Scan dependencies for known vulnerabilities
	cd api && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	cd web && pnpm audit

fmt: ## Format Go code
	cd api && gofmt -w .

clean: ## Remove build output
	rm -rf api/bin web/dist

