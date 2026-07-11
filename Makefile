export CGO_ENABLED := 0
export GOEXPERIMENT := jsonv2

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION := v2.12.2

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
REVISION := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GOBUILDFLAGS := -trimpath -ldflags="-s -w -X main.version=$(VERSION) -X main.revision=$(REVISION)"

.PHONY: build
build: OUTPUT ?= bin/decolint
build:
	go build $(GOBUILDFLAGS) -o $(OUTPUT) ./cmd/decolint

.PHONY: run
run: build
	./bin/decolint $(ARGS)

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

.PHONY: clean
clean:
	rm -rf bin

.PHONY: install
install:
	go install $(GOBUILDFLAGS) ./cmd/decolint
