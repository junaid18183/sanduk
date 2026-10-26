BINARY  := sanduk
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/junaid18183/sanduk/cli.version=$(VERSION)

# Pinned so `make lint vuln` matches CI (.github/workflows/ci.yml).
GOLANGCI_LINT_VERSION := v2.14.0
GOVULNCHECK_VERSION   := v1.8.0

.PHONY: build install test race cover vet fmt fmt-check lint vuln tidy-check cross check clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

test:
	go test ./...

race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

# fmt rewrites files; fmt-check only reports, as CI does.
fmt:
	gofmt -l -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt'd:"; echo "$$out"; exit 1; fi

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# Fails if `go mod tidy` would change go.mod or go.sum, without touching them.
tidy-check:
	@tmp=$$(mktemp -d); cp go.mod go.sum $$tmp/; \
	go mod tidy; \
	if ! cmp -s go.mod $$tmp/go.mod || ! cmp -s go.sum $$tmp/go.sum; then \
		cp $$tmp/go.mod $$tmp/go.sum .; rm -rf $$tmp; \
		echo "go.mod/go.sum are not tidy; run go mod tidy"; exit 1; \
	fi; rm -rf $$tmp

cross:
	@for t in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
		GOOS=$${t%/*} GOARCH=$${t#*/} CGO_ENABLED=0 go build -o /dev/null . || exit 1; \
		echo "built $$t"; \
	done

# Everything CI checks, in one go.
check: fmt-check tidy-check vet race lint vuln cross

# Everything the build generates, which is everything .gitignore lists.
clean:
	rm -rf $(BINARY) coverage.out dist *.test
