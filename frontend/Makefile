PNPM ?= pnpm

.PHONY: install
install:
	$(PNPM) install

API_BASE_URL ?= http://localhost:8080

.PHONY: dev
dev:
	NEXT_PUBLIC_API_BASE_URL=$(API_BASE_URL) $(PNPM) dev

.PHONY: build
build:
	$(PNPM) build

.PHONY: lint
lint:
	$(PNPM) lint

.PHONY: test
test:
	$(PNPM) test
