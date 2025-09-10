.PHONY: build test lint race bench clean help \
        test-logs-up test-logs-down test-logs-logs test-logs-build test-logs-run

# Default target
help:
	@echo "Available targets:"
	@echo "  build    - Build the siftail binary"
	@echo "  test     - Run unit tests"
	@echo "  lint     - Run static analysis with staticcheck"
	@echo "  race     - Run tests with race detector"
	@echo "  bench    - Run benchmarks"
	@echo "  clean    - Remove build artifacts"
	@echo "  test-logs-build - Build the test log generator image"
	@echo "  test-logs-run   - Run a single test log generator (env MODE=... SERVICE_NAME=...)"
	@echo "  test-logs-up    - Start compose stack with text/json/mixed generators"
	@echo "  test-logs-logs  - Tail compose logs"
	@echo "  test-logs-down  - Stop and remove the compose stack"

# Build the binary
build:
	go build -o siftail ./cmd/siftail

# Run tests
test:
	go test ./...

# Run static analysis
lint:
	@which staticcheck > /dev/null || (echo "staticcheck not found. Install with: go install honnef.co/go/tools/cmd/staticcheck@latest" && exit 1)
	staticcheck ./...
	go vet ./...

# Run tests with race detector
race:
	go test -race ./...

# Run benchmarks
bench:
	go test -bench=. ./...

# Clean build artifacts
clean:
	rm -f siftail
	go clean

# ---- Test log generators (Docker) ----

# Prefer Docker Compose v2 plugin if available, else fall back to docker-compose
COMPOSE := $(shell if docker compose version >/dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)

test-logs-build:
	docker build -t siftail-loggen ./testdata/loggen

# Example: make test-logs-run MODE=json SERVICE_NAME=api
test-logs-run: test-logs-build
	docker run --rm -e MODE=$(MODE) -e SERVICE_NAME=$(SERVICE_NAME) siftail-loggen

test-logs-up:
	$(COMPOSE) -f testdata/docker-compose.yml up --build -d

test-logs-logs:
	$(COMPOSE) -f testdata/docker-compose.yml logs -f

test-logs-down:
	$(COMPOSE) -f testdata/docker-compose.yml down --remove-orphans
