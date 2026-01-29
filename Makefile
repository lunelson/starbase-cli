.PHONY: build test lint clean install

BINARY := starbase
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/starbase

test:
	go test ./... -v

lint:
	golangci-lint run

clean:
	rm -rf bin/

install: build
	cp bin/$(BINARY) $(GOPATH)/bin/
