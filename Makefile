GO ?= go

.PHONY: test build web race
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
build:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/packmate ./cmd/packmate
web:
	cd web && npm ci --no-audit --no-fund && npm run build
