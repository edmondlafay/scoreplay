.PHONY: build test test-integration lint run docker-up docker-down seed smoke load stress

# ── Build ────────────────────────────────────────────────────────────────────

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/scoreplay ./cmd/api
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/seed       ./cmd/seed

# ── Tests ────────────────────────────────────────────────────────────────────

test:
	go test -race ./...

# Requires Docker / OrbStack running
test-integration:
	go test -tags integration -count=1 -race ./internal/repository/...

# ── Lint ─────────────────────────────────────────────────────────────────────

lint:
	go vet ./...
	@which golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

# ── Run ──────────────────────────────────────────────────────────────────────

run:
	go run ./cmd/api

# Provision a client and print the API key.
# Usage: make seed CLIENT="My Client"
seed:
	go run ./cmd/seed -name "$(CLIENT)"

# ── Docker ───────────────────────────────────────────────────────────────────

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

# ── Load tests (requires k6 and API_KEY env var) ─────────────────────────────

smoke:
	k6 run -e API_KEY=$(API_KEY) loadtest/k6/smoke.js

load:
	k6 run -e API_KEY=$(API_KEY) loadtest/k6/load.js

stress:
	k6 run -e API_KEY=$(API_KEY) loadtest/k6/stress.js
