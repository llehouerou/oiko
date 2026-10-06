# Run from the dev shell (direnv, or `nix develop`). The Bridges are in
# data/config.json.

.PHONY: dev build test icons

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
	cd web && npm test && npm run build && npm run test:browser

# The PNGs iOS and the web manifest want, from web/public/favicon.svg and web/icons/; they are
# committed, made again after a change to one of those.
icons:
	cd web && \
	magick -background none -density 1152 public/favicon.svg -resize 192x192 -strip -depth 8 public/icon-192.png && \
	magick -background none -density 1152 public/favicon.svg -resize 512x512 -strip -depth 8 public/icon-512.png && \
	magick -density 1152 icons/maskable.svg -resize 512x512 -strip -depth 8 public/icon-maskable.png && \
	magick -density 1152 icons/full-bleed.svg -resize 180x180 -strip -depth 8 public/apple-touch-icon.png

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch $@
