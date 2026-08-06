SHELL := /bin/sh
GOLANGCI_LINT_CACHE := $(CURDIR)/.tmp/golangci-lint-cache
GOCACHE := $(CURDIR)/.tmp/go-build
LINT_PACKAGES := ./cmd/helm-bom ./internal/cli ./internal/helm ./internal/images ./internal/sbom ./test/e2e

.PHONY: help check-tools build test test-e2e fmt fmt-check lint clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check-tools: ## Verify required external tools are installed
	@command -v helm >/dev/null 2>&1 || { echo "missing required tool: helm"; exit 1; }
	@helm version >/dev/null

build: check-tools ## Build the helm-bom binary
	go build -o bin/helm-bom ./cmd/helm-bom

test: check-tools ## Run all Go tests
	go test ./...

test-e2e: check-tools ## Run only the end-to-end integration test
	go test ./test/e2e -run TestHelmBOMExtractsImagesFromChart

fmt: ## Format Go code
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check: ## Check that Go code is formatted
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

lint: ## Run the linter
	go tool golangci-lint run

clean: ## Remove build artifacts
	rm -rf bin .tmp
