BINARY_NAME=ralph
CMD_PATH=./cmd/ralph
GIT_COMMIT := $(shell git rev-parse --short HEAD)
GOBIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

.PHONY: all build install test test-integration test-integration-basic test-integration-builds-once test-integration-builds-git test-integration-doctor-report test-integration-apply-report test-integration-dryrun test-integration-list-packages test-integration-apply-packages test-integration-cleanup-delete test-integration-cleanup-abandon test-integration-cleanup-safety test-integration-idempotent-skip test-integration-disable-cleanup lint format format-check clean sandbox

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@go build -ldflags "-X github.com/mad01/ralph/cmd/ralph/commands.Version=$(GIT_COMMIT)" -o $(BINARY_NAME) $(CMD_PATH)
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
	@./tests/integration/test_doctor_report/run_test.sh
	@./tests/integration/test_apply_report/run_test.sh
	@./tests/integration/test_dryrun/run_test.sh
	@./tests/integration/test_list_packages/run_test.sh
	@./tests/integration/test_apply_packages/run_test.sh
	@./tests/integration/test_idempotent_skip/run_test.sh
	@./tests/integration/test_cleanup_delete/run_test.sh
	@./tests/integration/test_cleanup_abandon/run_test.sh
	@./tests/integration/test_cleanup_safety/run_test.sh
	@./tests/integration/test_disable_cleanup/run_test.sh

test-integration-basic:
	@echo "Running basic apply integration test..."
	@./tests/integration/test_apply_basic/run_test.sh

test-integration-builds-once:
	@echo "Running builds idempotency integration test..."
	@./tests/integration/test_builds_once/run_test.sh

test-integration-builds-git:
	@echo "Running builds git change detection integration test..."
	@./tests/integration/test_builds_git/run_test.sh

test-integration-doctor-report:
	@echo "Running doctor report integration test..."
	@./tests/integration/test_doctor_report/run_test.sh

test-integration-apply-report:
	@echo "Running apply report integration test..."
	@./tests/integration/test_apply_report/run_test.sh

test-integration-dryrun:
	@echo "Running dry-run integration test..."
	@./tests/integration/test_dryrun/run_test.sh

test-integration-list-packages:
	@echo "Running list packages integration test..."
	@./tests/integration/test_list_packages/run_test.sh

test-integration-apply-packages:
	@echo "Running apply packages integration test..."
	@./tests/integration/test_apply_packages/run_test.sh

test-integration-idempotent-skip:
	@echo "Running idempotent build skip integration test..."
	@./tests/integration/test_idempotent_skip/run_test.sh

test-integration-disable-cleanup:
	@echo "Running disable + cleanup integration test..."
	@./tests/integration/test_disable_cleanup/run_test.sh

test-integration-cleanup-delete:
	@echo "Running cleanup delete integration test..."
	@./tests/integration/test_cleanup_delete/run_test.sh

test-integration-cleanup-abandon:
	@echo "Running cleanup abandon integration test..."
	@./tests/integration/test_cleanup_abandon/run_test.sh

test-integration-cleanup-safety:
	@echo "Running cleanup safety rails integration test..."
	@./tests/integration/test_cleanup_safety/run_test.sh

sandbox:
	@echo "Building and starting interactive ralph sandbox environment..."
	@docker build -t ralph-integration-test -f Dockerfile .
	@docker build -t ralph-sandbox -f Dockerfile.sandbox .
	@echo "Starting sandbox container. Type 'exit' when done."
	@docker run -it --rm ralph-sandbox

lint:
	@echo "Running linter (golangci-lint)..."
	@golangci-lint run ./...

format:
	@echo "Formatting code (goimports and gofmt)..."
	@goimports -w .
	@gofmt -w .

format-check:
	@echo "Checking formatting (gofmt)..."
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt-formatted:"; \
		echo "$$unformatted"; \
		echo "Run 'make format' to fix."; \
		exit 1; \
	fi
	@echo "All files are gofmt-formatted."

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
	@echo "  test-integration-doctor-report - Run doctor report integration test"
	@echo "  test-integration-apply-report  - Run apply report integration test"
	@echo "  test-integration-dryrun   - Run dry-run integration test"
	@echo "  test-integration-list-packages - Run list packages integration test"
	@echo "  test-integration-apply-packages - Run apply packages integration test"
	@echo "  test-integration-disable-cleanup - Run disable + cleanup integration test"
	@echo "  sandbox                   - Start an interactive ralph sandbox environment"
	@echo "  lint                      - Run golangci-lint (requires it to be installed)"
	@echo "  format                    - Format code using goimports and gofmt"
	@echo "  clean                     - Remove built binary and clean Go cache"
	@echo "  install_deps              - Install development dependencies"
	@echo "  help                      - Show this help message" 