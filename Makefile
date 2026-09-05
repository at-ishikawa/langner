OPENAI_API_KEY ?=
GEMINI_API_KEY ?=
INFERENCE_MODE ?=
API_BASE_URL ?= http://localhost:8080
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

# check-api-key guards the dev targets, requiring the API key for the selected
# INFERENCE_MODE (default openai): mock needs no key, gemini needs
# GEMINI_API_KEY, openai/unset needs OPENAI_API_KEY.
.PHONY: check-api-key
check-api-key:
	@case "$(INFERENCE_MODE)" in \
		mock) ;; \
		gemini) \
			if [ -z "$(GEMINI_API_KEY)" ]; then \
				echo "ERROR: GEMINI_API_KEY is not set (INFERENCE_MODE=gemini)"; \
				exit 1; \
			fi ;; \
		*) \
			if [ -z "$(OPENAI_API_KEY)" ]; then \
				echo "ERROR: OPENAI_API_KEY is not set"; \
				exit 1; \
			fi ;; \
	esac

.PHONY: dev-backend
dev-backend: check-api-key
	$(MAKE) -C backend build
	./langner-server

.PHONY: dev-frontend
dev-frontend:
	$(MAKE) -C frontend dev API_BASE_URL=$(API_BASE_URL)

BUF_VERSION ?= v1.66.0

.PHONY: dev
dev: check-api-key
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
