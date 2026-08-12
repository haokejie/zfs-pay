APP := zfs-pay
HELPER := zfs-pay-zed
GO ?= go
VERSION ?= dev
PACKAGE_VERSION ?= 0.1.0~dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

.PHONY: all build build-linux clean fmt fmt-check package package-test test vet verify

all: verify build

build:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP) ./cmd/$(APP)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w" -o dist/$(HELPER) ./cmd/$(HELPER)

build-linux:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-linux-amd64 ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(APP)-linux-arm64 ./cmd/$(APP)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/$(HELPER)-linux-amd64 ./cmd/$(HELPER)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/$(HELPER)-linux-arm64 ./cmd/$(HELPER)

package:
	VERSION=$(PACKAGE_VERSION) ./scripts/package/build-packages.sh

package-test:
	./scripts/package/test-lifecycle.sh

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$(gofmt -l $$(find . -type f -name '*.go' -not -path './.git/*'))" || \
		{ gofmt -l $$(find . -type f -name '*.go' -not -path './.git/*'); exit 1; }

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

verify: fmt-check test vet

clean:
	rm -rf dist
