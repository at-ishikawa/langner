GO ?= go
GOPATH_FIRST := $(shell $(GO) env GOPATH | cut -d: -f1)
BIN_DIR := $(shell $(GO) env GOBIN)
ifeq ($(BIN_DIR),)
	BIN_DIR := $(GOPATH_FIRST)/bin
endif

MIGRATE_BIN := $(BIN_DIR)/migrate
LINT_BIN := $(BIN_DIR)/golangci-lint

.PHONY: generate
generate:
	$(GO) generate ./...

.PHONY: build
build:
	$(GO) build -o ../langner-server ./cmd/langner-server

.PHONY: test
test:
	$(GO) test ./...

.PHONY: test-coverage
test-coverage:
	$(GO) test -coverprofile=coverage.out $$($(GO) list ./... | grep -v -e /gen-protos/ -e /internal/mocks/)
	@COVERAGE=$$($(GO) tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Total coverage: $${COVERAGE}%"


.PHONY: validate
validate:
	"$(LINT_BIN)" run
	$(GO) run ./cmd/langner validate

.PHONY: fix
fix:
	"$(LINT_BIN)" run --fix
	$(GO) run ./cmd/langner validate --fix

.PHONY: db-migrate
db-migrate:
	"$(MIGRATE_BIN)" -source file://schemas/migrations -database "$(DATABASE_URL)" up

.PHONY: db-import
db-import:
	$(GO) build -o ../langner ./cmd/langner
	cd .. && ./langner migrate import-db --config config.example.yml

.PHONY: install-tools
install-tools:
	$(GO) install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GO) install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
