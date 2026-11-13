BINARY_NAME=ralph
CMD_PATH=./cmd/ralph
GOBIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

# Build metadata linked into the binary and reported by `ralph version -o json`.
# Each shells out defensively: outside a git checkout (a source tarball, a
# container build context) the value is empty and the binary says so.
BUILDINFO_PKG := github.com/mad01/ralph/internal/buildinfo
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null)
GIT_FULL_COMMIT := $(shell git rev-parse HEAD 2>/dev/null)
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(BUILDINFO_PKG).Version=$(GIT_COMMIT) \
	-X $(BUILDINFO_PKG).Commit=$(GIT_FULL_COMMIT) \
	-X $(BUILDINFO_PKG).Tag=$(GIT_TAG) \
	-X $(BUILDINFO_PKG).BuildTime=$(BUILD_TIME)

# Pinned development tools. CI installs the formatters with `make
# install_format_deps` and reads GOLANGCI_LINT_VERSION for its lint step, so a
# bump here moves local and CI runs together.
GOIMPORTS_VERSION := v0.52.0
GOLINES_VERSION := v0.13.0
GOFUMPT_VERSION := v0.12.0
GOLANGCI_LINT_VERSION := v2.15.0
FORMAT_TOOLS := goimports golines gofumpt

.PHONY: all build install test test-integration test-integration-basic test-integration-builds-once test-integration-builds-git test-integration-doctor-report test-integration-apply-report test-integration-dryrun test-integration-list-packages test-integration-apply-packages test-integration-cleanup-delete test-integration-cleanup-abandon test-integration-cleanup-safety test-integration-idempotent-skip test-integration-disable-cleanup lint format format-check check-format-tools clean sandbox install_deps install_format_deps install_lint_deps golangci-lint-version help

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) $(CMD_PATH)
	@echo "$(BINARY_NAME) built successfully."

install: build
	@echo "Installing $(BINARY_NAME) to $(GOBIN)..."
	@mkdir -p $(GOBIN)
	@cp $(BINARY_NAME) $(GOBIN)/$(BINARY_NAME)
	@# macOS (Tahoe+) SIGKILLs a copied linker-signed binary; re-sign ad-hoc.
	@if [ "$$(uname)" = "Darwin" ]; then \
		xattr -dr com.apple.provenance $(GOBIN)/$(BINARY_NAME) 2>/dev/null || true; \
		codesign -fs - $(GOBIN)/$(BINARY_NAME); \
	fi
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

# golines silently skips its gofumpt pass when gofumpt is missing, so a check
# without the tools would report a clean tree. Fail instead.
check-format-tools:
	@missing=""; \
	for tool in $(FORMAT_TOOLS); do \
		command -v $$tool >/dev/null 2>&1 || missing="$$missing $$tool"; \
	done; \
	if [ -n "$$missing" ]; then \
		echo "Missing formatter(s):$$missing" >&2; \
		echo "Run 'make install_format_deps' and make sure $(GOBIN) is on PATH." >&2; \
		exit 1; \
	fi

# The formatters get an explicit file list. Given ".", golines skips every file
# when the checkout sits under a dot directory (a git worktree in
# ~/.worktrees), and the check then passes on unformatted code.
GO_FILES = $(shell git ls-files --cached --others --exclude-standard '*.go')

format: check-format-tools
	@echo "Formatting code (goimports + golines @100, gofumpt base)..."
	@goimports -w $(GO_FILES)
	@golines -m 100 --base-formatter=gofumpt -w $(GO_FILES)

format-check: check-format-tools
	@echo "Checking formatting (goimports + golines @100, gofumpt base)..."
	@imports=$$(goimports -l $(GO_FILES)) || exit 1; \
	lines=$$(golines -m 100 --base-formatter=gofumpt -l $(GO_FILES)) || exit 1; \
	unformatted=$$(printf '%s\n%s\n' "$$imports" "$$lines" | sort -u | sed '/^$$/d'); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not formatted:"; \
		echo "$$unformatted"; \
		echo "Run 'make format' to fix."; \
		exit 1; \
	fi
	@echo "All files are formatted."

clean:
	@echo "Cleaning up..."
	@rm -f $(BINARY_NAME)
	@go clean

install_deps: install_format_deps install_lint_deps

install_format_deps:
	@echo "Installing formatters (goimports $(GOIMPORTS_VERSION), golines $(GOLINES_VERSION), gofumpt $(GOFUMPT_VERSION))..."
	@go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	@go install github.com/segmentio/golines@$(GOLINES_VERSION)
	@go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

install_lint_deps:
	@echo "Installing linter (golangci-lint $(GOLANGCI_LINT_VERSION))..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

golangci-lint-version:
	@echo $(GOLANGCI_LINT_VERSION)

help:
	@echo "Available targets:"
	@echo "  all                              - Build the binary (default)"
	@echo "  build                            - Build the binary"
	@echo "  install                          - Build and install to GOBIN"
	@echo "  test                             - Run unit tests"
	@echo "  test-integration                 - Run all Docker-based integration tests"
	@echo "  test-integration-basic           - Run basic apply integration test"
	@echo "  test-integration-builds-once     - Run builds idempotency test"
	@echo "  test-integration-builds-git      - Run builds git change detection test"
	@echo "  test-integration-doctor-report   - Run doctor report integration test"
	@echo "  test-integration-apply-report    - Run apply report integration test"
	@echo "  test-integration-dryrun          - Run dry-run integration test"
	@echo "  test-integration-list-packages   - Run list packages integration test"
	@echo "  test-integration-apply-packages  - Run apply packages integration test"
	@echo "  test-integration-idempotent-skip - Run idempotent build skip integration test"
	@echo "  test-integration-disable-cleanup - Run disable + cleanup integration test"
	@echo "  test-integration-cleanup-delete  - Run cleanup delete integration test"
	@echo "  test-integration-cleanup-abandon - Run cleanup abandon integration test"
	@echo "  test-integration-cleanup-safety  - Run cleanup safety rails integration test"
	@echo "  sandbox                          - Start an interactive ralph sandbox environment"
	@echo "  lint                             - Run golangci-lint (install with install_lint_deps)"
	@echo "  format                           - Format code using goimports + golines (100, gofumpt base)"
	@echo "  format-check                     - Fail if any file needs 'make format' (or a formatter is missing)"
	@echo "  clean                            - Remove built binary and clean Go cache"
	@echo "  install_deps                     - Install the pinned formatters and linter"
	@echo "  install_format_deps              - Install the pinned goimports, golines and gofumpt"
	@echo "  install_lint_deps                - Install the pinned golangci-lint"
	@echo "  golangci-lint-version            - Print the pinned golangci-lint version (read by CI)"
	@echo "  help                             - Show this help message"