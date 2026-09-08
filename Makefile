.PHONY: run build ui contracts check browser-install test-e2e test-chrome browser-ui browser-mcp

contracts:
	go run ./cmd/contracts

web/node_modules/.package-lock.json: web/package.json web/package-lock.json
	npm --prefix web ci

ui: web/node_modules/.package-lock.json
	npm --prefix web run build
	touch web/dist/.gitkeep

run: ui
	go run ./cmd/server

build: ui
	go run ./cmd/contracts -check
	go build -o bin/ai-of-empires ./cmd/server

check: web/node_modules/.package-lock.json
	go run ./cmd/contracts -check
	npm --prefix web run typecheck
	go vet ./...
	go test -race ./...

browser-install: web/node_modules/.package-lock.json
	npm --prefix web run browser:install

test-e2e: web/node_modules/.package-lock.json
	npm --prefix web run test:e2e

test-chrome: web/node_modules/.package-lock.json
	npm --prefix web run test:chrome

browser-ui: web/node_modules/.package-lock.json
	npm --prefix web run test:ui

browser-mcp: web/node_modules/.package-lock.json
	npm --silent --prefix web run browser:mcp
