BINARY  := zap
PKG     := github.com/thecloudranger/zap
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION) \
           -X $(PKG)/internal/version.Commit=$(COMMIT) \
           -X $(PKG)/internal/version.Date=$(DATE)
TARGETS := darwin/arm64 darwin/amd64 linux/arm64 linux/amd64

export CGO_ENABLED=0

.PHONY: help build install test check fmt tidy-check vet lint test-race cross clean

help: ## Show targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  %-12s %s\n",$$1,$$2}'

build: ## Build ./zap
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/zap

install: ## Install to $GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/zap

test: ## Run tests
	go test ./...

check: fmt tidy-check vet lint test-race cross ## All quality gates (what CI runs)

fmt: ## Fail if any file is not gofmt'd
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

tidy-check: ## Fail if go.mod/go.sum are not tidy
	go mod tidy -diff

vet: ## go vet
	go vet ./...

lint: ## golangci-lint
	golangci-lint run

test-race: ## Tests with the race detector (needs cgo)
	CGO_ENABLED=1 go test -race ./...

cross: ## Cross-compile every release target
	@for t in $(TARGETS); do \
		echo "build $$t"; \
		GOOS=$${t%/*} GOARCH=$${t#*/} go build -trimpath -o /dev/null ./cmd/zap || exit 1; \
	done

clean: ## Remove build output
	rm -rf $(BINARY) dist
