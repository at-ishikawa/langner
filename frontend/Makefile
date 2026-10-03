PNPM ?= pnpm

.PHONY: install
install:
	$(PNPM) install

API_BASE_URL ?= http://localhost:8080
# AUTH_PROXY_TARGET is the SERVER-SIDE backend origin the Next dev server proxies
# the cookie-bearing /auth endpoints to (see next.config.ts). It must be
# reachable from the Next PROCESS — keep it localhost even when API_BASE_URL is a
# browser-only host (e.g. WSL's wsl.internal.net), which the Next server can't route to.
AUTH_PROXY_TARGET ?= http://localhost:8080

# `make dev` uses the webpack bundler, NOT Turbopack. Turbopack keeps a
# native on-disk cache and panics with "Permission denied (os error 13)" on
# filesystems that don't allow its locking/mmap (container overlays, WSL /
# networked mounts) — so `next dev` (Turbopack is Next 16's default) can't be
# relied on across environments. Webpack has no such dependency. CI's e2e job
# invokes `pnpm dev` (Turbopack) directly and runs on a plain FS, so it's
# unaffected; only local `make dev` is pinned to webpack here.
.PHONY: dev
dev:
	NEXT_PUBLIC_API_BASE_URL=$(API_BASE_URL) AUTH_PROXY_TARGET=$(AUTH_PROXY_TARGET) $(PNPM) run dev:webpack

.PHONY: build
build:
	$(PNPM) build

.PHONY: lint
lint:
	$(PNPM) lint

.PHONY: test
test:
	$(PNPM) test
