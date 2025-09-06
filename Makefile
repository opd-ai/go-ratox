# Makefile for ratox-go

# Build variables
BINARY_NAME=ratox-go
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Go build flags
GOFLAGS=-trimpath
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)

.PHONY: all build clean test lint fmt vet install uninstall run dev help

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME) v$(VERSION) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Vet code
vet:
	@echo "Vetting code..."
	@go vet ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# Uninstall the binary
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(GOPATH)/bin/$(BINARY_NAME)

# Run the application in development mode
run: build
	@echo "Running $(BINARY_NAME) in development mode..."
	@$(BUILD_DIR)/$(BINARY_NAME) -d -p ./dev-config

# Run the application with sample configuration
dev: build
	@echo "Setting up development environment..."
	@mkdir -p ./dev-config
	@$(BUILD_DIR)/$(BINARY_NAME) -d -p ./dev-config

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@GOOS=linux GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	@GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	@GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@GOOS=freebsd GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-freebsd-amd64 .
	@echo "Multi-platform build complete"

# Create release archive
release: build-all
	@echo "Creating release archives..."
	@mkdir -p $(BUILD_DIR)/releases
	@for binary in $(BUILD_DIR)/$(BINARY_NAME)-*; do \
		platform=$$(basename $$binary | sed 's/$(BINARY_NAME)-//'); \
		tar -czf $(BUILD_DIR)/releases/$(BINARY_NAME)-$(VERSION)-$$platform.tar.gz -C $(BUILD_DIR) $$(basename $$binary) -C .. README.md LICENSE; \
	done
	@echo "Release archives created in $(BUILD_DIR)/releases/"

# Check code quality
check: fmt vet lint test
	@echo "All checks passed!"

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  clean         - Clean build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint          - Run linter"
	@echo "  fmt           - Format code"
	@echo "  vet           - Vet code"
	@echo "  deps          - Install dependencies"
	@echo "  install       - Install binary to GOPATH/bin"
	@echo "  uninstall     - Remove binary from GOPATH/bin"
	@echo "  run           - Run in development mode"
	@echo "  dev           - Set up and run development environment"
	@echo "  build-all     - Build for multiple platforms"
	@echo "  release       - Create release archives"
	@echo "  check         - Run all code quality checks"
	@echo "  help          - Show this help message"
