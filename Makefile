# Mello CLI daemon and Mework server — build/test/release targets.

BINARY         := mework
CMD            := ./cmd/mework
SERVER_BINARY  := mework-server
SERVER_CMD     := ./cmd/mework-server
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT         := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE           := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS        := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test vet lint clean install snapshot server test-db \
	build-shared build-server build-client build-sandbox build-all \
	test-shared test-server test-client test-sandbox test-all

build: build-mework build-mework-server

build-mework:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./client/cmd/mework

build-mework-server:
	go build -ldflags "$(LDFLAGS)" -o bin/$(SERVER_BINARY) ./server/cmd/mework-server

server: build-mework-server

MODULES := libs/shared libs/server libs/client libs/sandbox libs/tests libs/tools

test:
	@for mod in $(MODULES); do \
		echo "--- $$mod ---"; \
		cd $$mod && go test -p 1 ./... && cd ..; \
	done

vet:
	@for mod in $(MODULES); do \
		echo "--- $$mod ---"; \
		cd $$mod && go vet ./... && cd ..; \
	done

# Start a local postgres container for tests
test-db:
	docker run --name mework-test-db -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=mework_test -p 5432:5432 -d postgres:16-alpine 2>/dev/null || docker start mework-test-db

# Optional; requires golangci-lint to be installed.
lint:
	golangci-lint run ./... || echo "golangci-lint not installed; skipping"
	@echo "--- import-guard ---"
	go test ./tools/import-guard/...

install:
	go install -ldflags "$(LDFLAGS)" $(CMD)
	go install -ldflags "$(LDFLAGS)" $(SERVER_CMD)

# Cross-compile a local snapshot via goreleaser (no publish).
snapshot:
	goreleaser build --snapshot --clean

clean:
	rm -rf bin dist

# ---- Per-module build/test targets ----

build-shared:
	cd shared && go build ./...

build-server:
	cd server && go build ./...

build-client:
	cd client && go build ./...

build-sandbox:
	cd sandbox && go build ./...

build-all: build-shared build-server build-client build-sandbox

test-shared:
	cd shared && go test ./...

test-server:
	cd server && go test ./...

test-client:
	cd client && go test ./...

test-sandbox:
	cd sandbox && go test ./...

test-all: test-shared test-server test-client test-sandbox

# ---- OpenSpec ship-all shortcuts (thin wrappers; the real work runs via the
# slash command /opsx:ship-all → Workflow({ name: 'ship-all', args: ... }))
# These targets are convenience aliases for terminal users who prefer Make.

# Dry-run the batch ship (no commits). Surfaces the queue + per-change mode.
ship-dry-run:
	@echo "ship-all --dry-run — launches via /opsx:ship-all (slash command)."

# Batch-ship all ACTIVE changes through apply → ship → archive locally.
# Pass ARGS="--from c0008 --bump patch" etc. for partial runs.
ship-all:
	@echo "ship-all — launches via /opsx:ship-all (slash command). Pass ARGS=\"...\" to forward flags."

# Batch-ship a single change (whitelist). Pass SLUG="c0008-object-storage".
ship-one:
	@echo "ship-all --only $(SLUG) — launches via /opsx:ship-all (slash command)."

