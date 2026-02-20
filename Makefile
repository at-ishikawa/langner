OPENAI_API_KEY ?=

.PHONY: pre-commit
pre-commit: generate validate test

.PHONY: generate
generate: proto
	cd backend && go generate ./...

.PHONY: proto
proto:
	go run github.com/bufbuild/buf/cmd/buf@latest generate

.PHONY: fix
fix:
	cd backend && golangci-lint run --fix
	cd backend && go run ./cmd/langner validate --fix

.PHONY: validate
validate:
	cd backend && golangci-lint run
	cd backend && go run ./cmd/langner validate

.PHONY: test
test:
	cd backend && go test ./...

COVERAGE_THRESHOLD ?= 90

.PHONY: test-coverage
test-coverage:
	@cd backend && go test -coverprofile=coverage.out $$(go list ./... | grep -v -e /gen-protos/ -e /internal/mocks/)
	@cd backend && COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	COVERAGE_INT=$${COVERAGE%.*}; \
	echo "Total coverage: $${COVERAGE}%"; \
	echo "Threshold: $(COVERAGE_THRESHOLD)%"; \
	if [ "$$COVERAGE_INT" -lt "$(COVERAGE_THRESHOLD)" ]; then \
		echo "ERROR: Coverage $${COVERAGE}% is below threshold $(COVERAGE_THRESHOLD)%"; \
		exit 1; \
	fi; \
	echo "Coverage check passed!"

.PHONY: test-integration
test-integration:
	@echo "Running OpenAI integration tests..."
	@cd backend && OPENAI_API_KEY=$(OPENAI_API_KEY) \
		go test -v ./internal/inference/openai -run Integration -timeout 60s

.PHONY: frontend-install
frontend-install:
	cd frontend && pnpm install

.PHONY: docs-setup
docs-setup:
	git submodule update --init --recursive

.PHONY: docs-server
docs-server: docs-setup
	hugo server -s docs

DATABASE_URL ?= mysql://user:password@tcp(localhost:3306)/local?multiStatements=true

.PHONY: db-migrate
db-migrate:
	cd backend && migrate -source file://schemas/migrations -database "$(DATABASE_URL)" up
