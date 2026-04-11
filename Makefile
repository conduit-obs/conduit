.PHONY: build test test-coverage test-race run clean lint fmt docker-up docker-down migrate-up migrate-status seed generate-openapi release loadtest check-coverage

BINARY := conduit
VERSION := $(shell grep 'Version' internal/version/version.go | head -1 | grep -o '"[^"]*"' | tr -d '"')
LDFLAGS := -ldflags "-X github.com/conduit-obs/conduit/internal/version.Version=$(VERSION) -X github.com/conduit-obs/conduit/internal/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) -X github.com/conduit-obs/conduit/internal/version.GitCommit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/conduit

test:
	go test ./... -count=1

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

test-race:
	go test ./... -race -count=1

run: build
	./bin/$(BINARY) serve

clean:
	rm -rf bin/ coverage.out

lint:
	@command -v golangci-lint >/dev/null 2>&1 || echo "Install golangci-lint: https://golangci-lint.run/usage/install/"
	golangci-lint run ./... || true

fmt:
	gofmt -w -s .
	go mod tidy

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

migrate-up:
	go run ./cmd/conduit migrate up

migrate-status:
	go run ./cmd/conduit migrate status

seed:
	./scripts/seed.sh

generate-openapi:
	cp api/openapi.yaml internal/api/openapi.yaml
	@echo "OpenAPI spec synced to internal/api/"

demo:
	./scripts/demo.sh

release:
	@test -n "$(V)" || (echo "Usage: make release V=1.0.0" && exit 1)
	./scripts/release.sh $(V)

loadtest:
	./scripts/loadtest.sh

check-coverage:
	./scripts/check-coverage.sh
