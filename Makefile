OPENAI_API_KEY ?=

.PHONY: pre-commit
pre-commit: generate validate test

.PHONY: generate
generate:
	go generate ./...

.PHONY: fix
fix:
	golangci-lint run --fix
	go run ./cmd/langner validate --fix

.PHONY: validate
validate:
	golangci-lint run
	go run ./cmd/langner validate

.PHONY: test
test:
	go test ./...

COVERAGE_THRESHOLD ?= 90

.PHONY: test-coverage
test-coverage:
	@go test -coverprofile=coverage.out ./...
	@grep -v 'internal/mocks/' coverage.out > coverage.filtered.out
	@COVERAGE=$$(go tool cover -func=coverage.filtered.out | grep total | awk '{print $$3}' | sed 's/%//'); \
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
	@OPENAI_API_KEY=$(OPENAI_API_KEY) \
		go test -v ./internal/inference/openai -run Integration -timeout 60s

.PHONY: docs-setup
docs-setup:
	git submodule update --init --recursive

.PHONY: docs-server
docs-server: docs-setup
	hugo server -s docs

DATABASE_URL ?= mysql://user:password@tcp(localhost:3306)/local?multiStatements=true

.PHONY: db-migrate
db-migrate:
	migrate -source file://schemas/migrations -database "$(DATABASE_URL)" up

.PHONY: db-import
db-import:
	go run ./cmd/langner migrate import-db --config config.example.yml
