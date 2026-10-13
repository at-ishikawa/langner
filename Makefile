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
# database and launch the server. This repo is PUBLIC, so `make dev` NEVER
# generates or commits secrets — you supply them yourself:
#   - Real grading (default, INFERENCE_MODE=user): set TOKEN_SIGNING_KEY and
#     CREDENTIAL_ENCRYPTION_KEY as the token_signing_key / credential_encryption_key
#     keys in a gitignored config.yml (copy config.example.yml), or export them in
#     your environment. Generate each with `openssl rand -hex 32`; they MUST
#     differ. If your dev server points at a SHARED/prod database these MUST equal
#     that deployment's values, or decrypt-on-read of stored per-user keys fails
#     with "cipher: message authentication failed".
#   - Offline grader: `INFERENCE_MODE=mock make dev` needs NO secrets — mock mode
#     supplies fixed dev-only keys and grades with a deterministic matcher.
#
# To actually SIGN IN you must also supply your own Google OAuth credentials:
# google_client_id, redirect_url, frontend_url, and allowed_emails in your
# config file (see the auth: block in config.example.yml), plus
# GOOGLE_CLIENT_SECRET exported in your environment.
#
# Grading is PER-USER: there is no system OPENAI_API_KEY/GEMINI_API_KEY. Register
# your own provider key in the app's Settings page (CREDENTIAL_ENCRYPTION_KEY
# backs that).

.PHONY: dev-backend
dev-backend:
	docker compose up -d --wait
	$(MAKE) -C backend build
	./langner-server

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
