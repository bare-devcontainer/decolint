export CGO_ENABLED := 0
export GOEXPERIMENT := jsonv2

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION := v2.12.2

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GOBUILDFLAGS := -trimpath -ldflags="-s -w -X main.version=$(VERSION) -X main.revision=$(REVISION)"

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: OUTPUT ?= bin/decolint
build: ## Build the decolint binary
	go build $(GOBUILDFLAGS) -o $(OUTPUT) ./cmd/decolint

.PHONY: run
run: build ## Build and run the decolint binary
	./bin/decolint $(ARGS)

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: lint
lint: ## Run all lint rules
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin

.PHONY: install
install: ## Install the decolint binary to GOPATH/bin
	go install $(GOBUILDFLAGS) ./cmd/decolint
