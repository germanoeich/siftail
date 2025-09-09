.PHONY: build test lint race bench clean help

# Default target
help:
	@echo "Available targets:"
	@echo "  build    - Build the siftail binary"
	@echo "  test     - Run unit tests"
	@echo "  lint     - Run static analysis with staticcheck"
	@echo "  race     - Run tests with race detector"
	@echo "  bench    - Run benchmarks"
	@echo "  clean    - Remove build artifacts"

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