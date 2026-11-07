BINARY  := ssterm
PKG     := ./cmd/ssterm
GO      ?= go

.PHONY: all build install test test-race golden vet fmt fmt-check check cross-build clean help

all: check build

## build: Compile the ssterm binary into the current directory
build:
	$(GO) build -o $(BINARY) $(PKG)

## install: Install ssterm into $(GOPATH)/bin or $(GOBIN)
install:
	$(GO) install $(PKG)

## test: Run all tests
test:
	$(GO) test ./...

## test-race: Run all tests with the race detector enabled
test-race:
	$(GO) test -race ./...

## golden: Regenerate SVG golden test files in internal/cli/testdata/
golden:
	$(GO) test ./internal/cli -update

## vet: Run go vet across all packages
vet:
	$(GO) vet ./...

## fmt: Format all Go source files with gofmt
fmt:
	gofmt -s -w .

## fmt-check: Verify all Go source files are formatted
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Unformatted files:" && gofmt -l . && exit 1)

## check: Run all quality gates (fmt-check, build, vet, test)
check: fmt-check
	$(GO) build ./...
	$(GO) vet ./...
	$(GO) test ./...

## cross-build: Verify compilation across Linux, macOS, and Windows
cross-build:
	GOOS=linux   GOARCH=amd64 $(GO) build ./...
	GOOS=linux   GOARCH=arm64 $(GO) build ./...
	GOOS=darwin  GOARCH=amd64 $(GO) build ./...
	GOOS=darwin  GOARCH=arm64 $(GO) build ./...
	GOOS=windows GOARCH=amd64 $(GO) build ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY) $(BINARY).exe

## help: Show available Make targets
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
