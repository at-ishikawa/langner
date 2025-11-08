OPENAI_API_KEY ?=

.PHONY: pre-commit
pre-commit: fix test

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

.PHONY: test-integration
test-integration:
	@echo "Running OpenAI integration tests..."
	@OPENAI_API_KEY=$(OPENAI_API_KEY) \
		go test -v ./internal/inference/openai -run Integration -timeout 60s
