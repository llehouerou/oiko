# Run from the dev shell (direnv, or `nix develop`). The Bridges are in
# data/config.json.

.PHONY: dev build test icons screenshots

# API on :8080, UI with hot reload on http://localhost:5180. Ctrl-C stops both.
dev: web/node_modules
	@trap 'kill 0' EXIT; \
	go run ./cmd/oiko & \
	cd web && npm run dev

build: web/node_modules
	cd web && npm run build
	go build -o oiko ./cmd/oiko

test: web/node_modules
	test -z "$$(gofmt -l .)" || { gofmt -l .; echo 'gofmt: the files above are not formatted'; exit 1; }
	go vet ./...
	scripts/check-licenses
	go test -race ./...
	cd docs/write-a-bridge/plug && go vet ./... && go test -race ./... # the guide's type, a module of its own
	cd web && npm run lint && npm test && npm run build && npm run test:browser

# The PNGs iOS and the web manifest want, and GitHub's social preview (kept out of public/), from
# web/public/favicon.svg and web/icons/; they are committed, made again after a change to one of those.
icons:
	cd web && \
	magick -background none -density 1152 public/favicon.svg -resize 192x192 -strip -depth 8 public/icon-192.png && \
	magick -background none -density 1152 public/favicon.svg -resize 512x512 -strip -depth 8 public/icon-512.png && \
	magick -density 1152 icons/maskable.svg -resize 512x512 -strip -depth 8 public/icon-maskable.png && \
	magick -density 1152 icons/full-bleed.svg -resize 180x180 -strip -depth 8 public/apple-touch-icon.png && \
	FONTCONFIG_FILE=$$ICON_FONTS magick icons/social-preview.svg -strip -depth 8 ../docs/images/social-preview.png

# docs/images/dashboard.png, the README's built-in Dashboard of a made-up home (web/e2e/home.json);
# committed, made again by whoever visibly changes the built-in Dashboard.
screenshots: web/node_modules
	cd web && npm run build && node e2e/screenshots.js

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch $@
