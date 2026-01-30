.PHONY: build test lint clean install install-local verify fmt

BINARY := starbase
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/starbase

test:
	go test ./... -v

test-short:
	go test ./... -short

lint:
	golangci-lint run

fmt:
	$$(go env GOPATH)/bin/goimports -w .

verify:
	./hack/verify_all.sh

clean:
	rm -rf bin/

# Install to GOPATH/bin (requires GOPATH set)
install: build
	@if [ -z "$(GOPATH)" ]; then echo "GOPATH not set, using go env GOPATH"; fi
	cp bin/$(BINARY) $$(go env GOPATH)/bin/

# Install to /usr/local/bin (requires sudo on macOS)
install-local: build
	cp bin/$(BINARY) /usr/local/bin/

# Install to ~/.local/bin (no sudo needed)
install-user: build
	mkdir -p ~/.local/bin
	cp bin/$(BINARY) ~/.local/bin/
	@echo "Ensure ~/.local/bin is in your PATH"
