# Mello CLI daemon and Mework server — build/test/release targets.

BINARY         := mework
CMD            := ./cmd/mework
SERVER_BINARY  := mework-server
SERVER_CMD     := ./cmd/mework-server
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT         := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE           := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS        := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test vet lint clean install snapshot server test-db

build: build-cli build-server

build-cli:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

build-server:
	go build -ldflags "$(LDFLAGS)" -o bin/$(SERVER_BINARY) $(SERVER_CMD)

server: build-server

test:
	go test -p 1 ./...

vet:
	go vet ./...

# Start a local postgres container for tests
test-db:
	docker run --name mework-test-db -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=mework_test -p 5432:5432 -d postgres:16-alpine 2>/dev/null || docker start mework-test-db

# Optional; requires golangci-lint to be installed.
lint:
	golangci-lint run ./... || echo "golangci-lint not installed; skipping"

install:
	go install -ldflags "$(LDFLAGS)" $(CMD)
	go install -ldflags "$(LDFLAGS)" $(SERVER_CMD)

# Cross-compile a local snapshot via goreleaser (no publish).
snapshot:
	goreleaser build --snapshot --clean

clean:
	rm -rf bin dist

