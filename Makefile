# Run from the dev shell (direnv, or `nix develop`). The Bridges are in
# data/config.json.

.PHONY: dev build test

# API on :8080, UI with hot reload on http://localhost:5180. Ctrl-C stops both.
dev: web/node_modules
	@trap 'kill 0' EXIT; \
	go run ./cmd/oiko & \
	cd web && npm run dev

build: web/node_modules
	cd web && npm run build
	go build -o oiko ./cmd/oiko

test: web/node_modules
	go test -race ./...
	cd web && npm test

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch $@
