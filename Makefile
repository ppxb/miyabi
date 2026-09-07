.PHONY: install generate dev web-build build test lint format-check

install:
	go get -tool entgo.io/ent/cmd/ent@latest
	go get entgo.io/ent@latest github.com/gin-gonic/gin@latest github.com/knadh/koanf/v2@latest github.com/knadh/koanf/parsers/toml@latest github.com/knadh/koanf/providers/confmap@latest github.com/knadh/koanf/providers/env@latest github.com/knadh/koanf/providers/file@latest github.com/samber/slog-gin@latest modernc.org/sqlite@latest golang.org/x/tools@latest
	pnpm --dir web install

generate:
	go generate ./internal/ent

dev:
	pnpm --dir web dev:all

web-build:
	pnpm --dir web build

build: web-build generate
	go build -o bin/miyabi ./cmd/miyabi

test: web-build generate
	go test ./...

lint: generate
	go vet ./...
	pnpm --dir web lint

format-check:
	pnpm --dir web fmt:check
