# Force the shell to bash for consistency.
export SHELL := /bin/bash

# Go parameters
PACKAGES := ./...
TEST_FLAGS := -v -race -cover
COVER_PROFILE := coverage.out

# Tool Binaries - allows overriding from the environment (e.g., make GOLANGCI_LINT=/path/to/binary lint)
GOLANGCI_LINT ?= golangci-lint

.DEFAULT_GOAL := help

# ====================================================================================
# HELPERS
# ====================================================================================

help: ## Show this help message.
	@echo "Usage: make <target>"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ====================================================================================
# QUALITY & TESTING (CI Targets)
# ====================================================================================

all: tidy fmt vet lint test ## Run all quality checks and tests.

ci: all ## Alias for 'all', intended for CI environments.

tidy: ## Tidy go.mod and go.sum files.
	@echo "==> Tidying module files..."
	@go mod tidy

fmt: ## Format Go source files.
	@echo "==> Formatting Go source files..."
	@go fmt $(PACKAGES)

vet: ## Vet Go source files for common issues.
	@echo "==> Vetting Go source files..."
	@go vet $(PACKAGES)

lint: ## Lint the codebase with golangci-lint.
	@echo "==> Linting Go source files..."
	@$(GOLANGCI_LINT) run $(PACKAGES)

test: ## Run tests with race detector and generate a coverage profile.
	@echo "==> Running tests with race detector and coverage..."
	@go test $(TEST_FLAGS) -coverprofile=$(COVER_PROFILE) $(PACKAGES)

# ====================================================================================
# DEVELOPMENT & UTILITIES
# ====================================================================================

update-deps: ## Update all Go module dependencies to the latest version.
	@echo "==> Updating dependencies..."
	@go get -u -t -v $(PACKAGES)
	@go mod tidy

cover-html: test ## Generate and view the HTML coverage report.
	@echo "==> Generating and opening HTML coverage report..."
	@go tool cover -html=$(COVER_PROFILE)

clean: ## Clean up build artifacts and coverage reports.
	@echo "==> Cleaning up artifacts..."
	@rm -f $(COVER_PROFILE)

tools: ## Install required development tools.
	@echo "==> Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: all lint update-deps test tidy fmt vet ci help cover-html clean tools
