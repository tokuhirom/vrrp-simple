.PHONY: all build test clean install uninstall integration-test test-all lxc-integration-test

# Variables
BINARY_NAME := vrrp
INSTALL_PATH := /usr/local/bin
GO := go
GOFLAGS := -v
BUILD_FLAGS := -ldflags="-s -w"

# Default target
all: build

# Build the binary
build:
	$(GO) build $(GOFLAGS) $(BUILD_FLAGS) -o $(BINARY_NAME) ./main.go

# Run unit tests
test:
	$(GO) test $(GOFLAGS) -race -cover ./...

# Run unit tests with coverage
test-coverage:
	$(GO) test $(GOFLAGS) -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run integration tests (requires root)
integration-test:
	@if [ "$$(id -u)" != "0" ]; then \
		echo "Integration tests must be run as root. Try: sudo make integration-test"; \
		exit 1; \
	fi
	@chmod +x test/integration/scripts/*.sh
	@./test/integration/scripts/run_integration_tests.sh

# Run LXC integration tests (REAL VIP testing with full network stack)
lxc-integration-test:
	@echo "Running LXC-based VIP movement integration tests..."
	@chmod +x test/lxc/run-lxc-tests.sh
	@sudo ./test/lxc/run-lxc-tests.sh

# Run all tests (unit + integration + lxc)
test-all: test
	@echo "Running namespace integration tests (requires root)..."
	@sudo $(MAKE) integration-test
	@echo "Running LXC VIP movement tests..."
	@$(MAKE) lxc-integration-test

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -f test/integration/*.log
	$(GO) clean

# Install binary to system
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)"
	@sudo cp $(BINARY_NAME) $(INSTALL_PATH)/
	@sudo chmod 755 $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Installation complete"

# Uninstall binary from system
uninstall:
	@echo "Removing $(BINARY_NAME) from $(INSTALL_PATH)"
	@sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Uninstall complete"

# Format code
fmt:
	$(GO) fmt ./...
	@echo "Code formatted"

# Run linters
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --no-config ./...; \
	else \
		echo "golangci-lint not installed. Install with:"; \
		echo "  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin"; \
	fi

# Run go vet
vet:
	$(GO) vet ./...

# Check for security issues
security:
	@if command -v gosec >/dev/null 2>&1; then \
		gosec -quiet ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
	fi

# Development mode - build and run with example config
dev: build
	sudo ./$(BINARY_NAME) run --interface lo --vrid 10 --priority 100 --vips 127.0.0.100

# LXC test environment setup (interactive)
lxc-setup:
	@echo "Running interactive LXC test setup..."
	@chmod +x test/lxc/setup-lxc-test.sh
	@sudo ./test/lxc/setup-lxc-test.sh

# Show help
help:
	@echo "Available targets:"
	@echo "  make build                 - Build the VRRP binary"
	@echo "  make test                  - Run unit tests"
	@echo "  make test-coverage         - Run tests with coverage report"
	@echo "  make integration-test      - Run namespace integration tests (requires root)"
	@echo "  make lxc-integration-test  - Run LXC VIP movement tests (requires root)"
	@echo "  make lxc-setup             - Interactive LXC test environment setup"
	@echo "  make test-all              - Run all tests (unit + integration + lxc)"
	@echo "  make clean                 - Remove build artifacts"
	@echo "  make install               - Install binary to system"
	@echo "  make uninstall             - Remove binary from system"
	@echo "  make fmt                   - Format Go code"
	@echo "  make lint                  - Run linters"
	@echo "  make vet                   - Run go vet"
	@echo "  make security              - Run security scanner"
	@echo "  make dev                   - Run in development mode"
	@echo "  make help                  - Show this help message"