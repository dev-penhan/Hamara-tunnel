SHELL := /bin/bash
VERSION ?= 0.2.0
BINARY := bin/hamara

.PHONY: all build test vet fmt check integration release install clean

all: check build

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/hamara

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

check:
	test -z "$$(gofmt -l ./cmd ./internal)"
	go vet ./...
	go test ./...
	bash -n install.sh
	bash -n scripts/quick-install.sh scripts/test-ara-transport.sh scripts/build-release.sh

integration:
	./scripts/test-ara-transport.sh

release: check
	./scripts/build-release.sh

install: build
	install -m 0755 $(BINARY) /usr/local/bin/hamara

clean:
	rm -rf bin
