API_BASE_URL ?= http://localhost:8080
# Server-side target the Next dev server proxies /auth to; defaults to a
# locally-reachable backend (see frontend/Makefile / next.config.ts).
AUTH_PROXY_TARGET ?= http://localhost:8080
DATABASE_URL ?= postgres://user:password@localhost:5432/local?sslmode=disable

.PHONY: pre-commit
pre-commit: generate validate test

.PHONY: generate
generate: proto
	$(MAKE) -C backend generate

.PHONY: setup
setup:
	docker compose up -d --wait
	$(MAKE) -C backend install-tools
	$(MAKE) -C frontend install
	$(MAKE) proto
	$(MAKE) db-migrate

# `make dev` runs with auth ALWAYS ON (there is no ungated mode). The server
# fails fast without TOKEN_SIGNING_KEY and requires Postgres, so we start the
# database and generate a DEV-ONLY signing key into a GITIGNORED file (this repo
# is public — never commit keys) and source it for the server.
#
# To actually SIGN IN you must also supply your own Google OAuth credentials:
# google_client_id, redirect_url, frontend_url, and allowed_emails in your
# config file (see the auth: block in config.example.yml), plus
# GOOGLE_CLIENT_SECRET exported in your environment.
#
# Grading is PER-USER: there is no system OPENAI_API_KEY/GEMINI_API_KEY. Register
# your own provider key in the app's Settings page (the dev CREDENTIAL_ENCRYPTION_KEY
# generated below backs that), or run `INFERENCE_MODE=mock make dev` for the
# deterministic offline grader.
AUTH_DEV_ENV := .dev-auth.env

.PHONY: dev-backend
dev-backend:
	docker compose up -d --wait
	$(MAKE) -C backend build
	@touch $(AUTH_DEV_ENV)
	@# Generate a DEV-ONLY key ONLY when you haven't supplied one (env or
	@# .dev-auth.env). If your dev server points at a SHARED/prod database, set
	@# both keys to that deployment's values — e.g. CREDENTIAL_ENCRYPTION_KEY must
	@# match or decrypt-on-read of existing per-user credentials fails — and this
	@# never overwrites them.
	@if [ -z "$$TOKEN_SIGNING_KEY" ] && ! grep -q '^TOKEN_SIGNING_KEY=' $(AUTH_DEV_ENV); then \
		printf 'TOKEN_SIGNING_KEY=%s\n' "$$(openssl rand -hex 32)" >> $(AUTH_DEV_ENV); \
		echo "Generated DEV-ONLY TOKEN_SIGNING_KEY -> $(AUTH_DEV_ENV) (gitignored)."; \
	fi
	@if [ -z "$$CREDENTIAL_ENCRYPTION_KEY" ] && ! grep -q '^CREDENTIAL_ENCRYPTION_KEY=' $(AUTH_DEV_ENV); then \
		printf 'CREDENTIAL_ENCRYPTION_KEY=%s\n' "$$(openssl rand -hex 32)" >> $(AUTH_DEV_ENV); \
		echo "Generated DEV-ONLY CREDENTIAL_ENCRYPTION_KEY -> $(AUTH_DEV_ENV) (gitignored)."; \
	fi
	@# .dev-auth.env is authoritative (it is sourced below), so to pin keys to a
	@# shared/prod deployment put them THERE. Google sign-in also needs
	@# google_client_id + allowed_emails in your config and GOOGLE_CLIENT_SECRET.
	set -a; . ./$(AUTH_DEV_ENV); set +a; ./langner-server

.PHONY: dev-frontend
dev-frontend:
	$(MAKE) -C frontend dev API_BASE_URL=$(API_BASE_URL) AUTH_PROXY_TARGET=$(AUTH_PROXY_TARGET)

BUF_VERSION ?= v1.66.0

.PHONY: dev
dev:
	$(MAKE) -j2 dev-backend dev-frontend

.PHONY: proto
proto:
	go run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

.PHONY: fix
fix:
	$(MAKE) -C backend fix

.PHONY: validate
validate:
	$(MAKE) -C backend validate

.PHONY: test
test:
	$(MAKE) -C backend test

.PHONY: test-coverage
test-coverage:
	$(MAKE) -C backend test-coverage

.PHONY: test-integration
test-integration:
	@echo "Running OpenAI integration tests..."
	@cd backend && OPENAI_API_KEY=$(OPENAI_API_KEY) \
		go test -v ./internal/inference/openai -run Integration -timeout 60s

.PHONY: db-migrate
db-migrate:
	$(MAKE) -C backend db-migrate DATABASE_URL="$(DATABASE_URL)"

.PHONY: db-import
db-import:
	$(MAKE) -C backend db-import

.PHONY: docs-setup
docs-setup:
	git submodule update --init --recursive

.PHONY: docs-server
docs-server: docs-setup
	hugo server -s docs --bind 0.0.0.0
