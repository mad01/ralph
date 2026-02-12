BINARY_NAME=dotter
CMD_PATH=./cmd/dotter
GIT_COMMIT := $(shell git rev-parse --short HEAD)
GOBIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

.PHONY: all build install test test-integration test-integration-basic test-integration-builds-once test-integration-builds-git lint format clean sandbox

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@go build -ldflags "-X github.com/mad01/dotter/cmd/dotter/commands.Version=$(GIT_COMMIT)" -o $(BINARY_NAME) $(CMD_PATH)
	@echo "$(BINARY_NAME) built successfully."

install: build
	@echo "Installing $(BINARY_NAME) to $(GOBIN)..."
	@mkdir -p $(GOBIN)
	@cp $(BINARY_NAME) $(GOBIN)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) installed to $(GOBIN)/$(BINARY_NAME)"

test:
	@echo "Running tests with 30s timeout..."
	@go test ./... -v -timeout 30s

test-integration:
	@echo "Running all integration tests..."
	@./tests/integration/test_apply_basic/run_test.sh
	@./tests/integration/test_builds_once/run_test.sh
	@./tests/integration/test_builds_git/run_test.sh

test-integration-basic:
	@echo "Running basic apply integration test..."
	@./tests/integration/test_apply_basic/run_test.sh

test-integration-builds-once:
	@echo "Running builds idempotency integration test..."
	@./tests/integration/test_builds_once/run_test.sh

test-integration-builds-git:
	@echo "Running builds git change detection integration test..."
	@./tests/integration/test_builds_git/run_test.sh

sandbox:
	@echo "Building and starting interactive dotter sandbox environment..."
	@docker build -t dotter-integration-test -f Dockerfile .
	@docker build -t dotter-sandbox -f Dockerfile.sandbox .
	@echo "Starting sandbox container. Type 'exit' when done."
	@docker run -it --rm dotter-sandbox

lint:
	@echo "Running linter (golangci-lint)..."
	@golangci-lint run ./...

format:
	@echo "Formatting code (goimports and gofmt)..."
	@goimports -w .
	@gofmt -w .

clean:
	@echo "Cleaning up..."
	@rm -f $(BINARY_NAME)
	@go clean

install_deps:
	@echo "Installing linter (golangci-lint) and formatter (goimports)..."
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

help:
	@echo "Available targets:"
	@echo "  all                       - Build the binary (default)"
	@echo "  build                     - Build the binary"
	@echo "  install                   - Build and install to GOBIN"
	@echo "  test                      - Run unit tests"
	@echo "  test-integration          - Run all Docker-based integration tests"
	@echo "  test-integration-basic    - Run basic apply integration test"
	@echo "  test-integration-builds-once - Run builds idempotency test"
	@echo "  test-integration-builds-git  - Run builds git change detection test"
	@echo "  sandbox                   - Start an interactive dotter sandbox environment"
	@echo "  lint                      - Run golangci-lint (requires it to be installed)"
	@echo "  format                    - Format code using goimports and gofmt"
	@echo "  clean                     - Remove built binary and clean Go cache"
	@echo "  install_deps              - Install development dependencies"
	@echo "  help                      - Show this help message" 