SHELL := /bin/sh
GOLANGCI_LINT_CACHE := $(CURDIR)/.tmp/golangci-lint-cache
GOCACHE := $(CURDIR)/.tmp/go-build
LINT_PACKAGES := ./cmd/bom ./internal/cli ./internal/helm ./internal/images ./internal/sbom ./test/e2e

.PHONY: help check-tools build test test-e2e fmt fmt-check lint dist clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check-tools: ## Verify required external tools are installed
	@command -v helm >/dev/null 2>&1 || { echo "missing required tool: helm"; exit 1; }
	@helm version >/dev/null

build: check-tools ## Build the bom binary
	go build -o bin/bom ./cmd/bom

build-linux: check-tools ## Build the bom binary
	GOOS=linux GOARCH=amd64 go build -o bin/bom ./cmd/bom

build-action: check-tools ## Build the bom binary
	go build -o bin/bom-action ./cmd/bom-action

build-action-linux: check-tools ## Build the bom binary
	GOOS=linux GOARCH=amd64 go build -o bin/bom-action ./cmd/bom-action

dist: ## Build release binaries for the default OS/arch matrix into dist/
	./scripts/build-dist.sh

test: check-tools ## Run all Go tests
	go test ./...

test-e2e: check-tools ## Run only the end-to-end integration test
	go test ./test/e2e -run TestBOMExtractsImagesFromChart

fmt: ## Format Go code
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check: ## Check that Go code is formatted
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

lint: ## Run the linter
	go tool golangci-lint run

clean: ## Remove build artifacts
	rm -rf bin dist .tmp

generate-license: generate
	go tool go-licenses report --template .NOTICE.template ./... > NOTICE
	copywrite headers apply

generate-notice:
	go tool go-licenses report --template .NOTICE.template ./... > NOTICE
