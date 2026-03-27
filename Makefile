.PHONY: build test clean python-setup lint lint-python test-python fix-check ci dist validate validate-fresh validate-ingest benchmark-extract benchmark-embedding changelog setup-hooks

BINARY := refloom
BUILD_DIR := bin
DIST_DIR := dist
GO_TAGS := fts5
CGO_ENABLED := 1

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X 'github.com/dora56/refloom/internal/cli.Version=$(VERSION)' \
           -X 'github.com/dora56/refloom/internal/cli.Commit=$(COMMIT)' \
           -X 'github.com/dora56/refloom/internal/cli.BuildDate=$(BUILD_DATE)'

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/refloom

test:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags "$(GO_TAGS)" ./...

clean:
	rm -rf $(BUILD_DIR) $(DIST_DIR)

python-setup:
	cd python/refloom_worker && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt

lint:
	golangci-lint run --build-tags "$(GO_TAGS)"

lint-python:
	cd python/refloom_worker && uv run --group dev ruff check . && uv run --group dev pyright .

test-python:
	cd python/refloom_worker && uv run --group dev pytest

fix-check:
	@diff=$$(go fix -diff -tags "$(GO_TAGS)" ./... 2>&1); \
	if [ -n "$$diff" ]; then \
		echo "go fix has pending modernizations:"; \
		echo "$$diff"; \
		echo "Run 'go fix -tags fts5 ./...' to apply."; \
		exit 1; \
	fi

ci: lint fix-check test lint-python test-python

setup-hooks:
	ln -sf ../../scripts/git/pre-commit .git/hooks/pre-commit
	ln -sf ../../scripts/git/commit-msg .git/hooks/commit-msg
	@echo "Git hooks installed."

changelog:
	git-cliff -o CHANGELOG.md

validate: build
	./scripts/validate_refloom.sh

validate-fresh: build
	VALIDATE_FRESH_DB=1 ./scripts/validate_refloom.sh

validate-ingest: build
	VALIDATE_FRESH_DB=1 VALIDATE_SKIP_INSPECT=1 VALIDATE_SKIP_SEARCH=1 VALIDATE_SKIP_ASK=1 VALIDATE_SKIP_SCORE=1 ./scripts/validate_refloom.sh

benchmark-extract: build
	./scripts/benchmark_extract.sh

benchmark-embedding: build
	./scripts/benchmark_embedding.sh

dist: build
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)/refloom/python/refloom_worker
	cp $(BUILD_DIR)/$(BINARY) $(DIST_DIR)/refloom/
	cp -r python/refloom_worker/*.py $(DIST_DIR)/refloom/python/refloom_worker/
	cp python/refloom_worker/pyproject.toml $(DIST_DIR)/refloom/python/refloom_worker/
	cp config/refloom.example.yaml $(DIST_DIR)/refloom/
	cd $(DIST_DIR) && zip -r refloom-$(VERSION)-darwin-arm64.zip refloom/
	@echo "Distribution created: $(DIST_DIR)/refloom-$(VERSION)-darwin-arm64.zip"
	@echo "SHA256: $$(shasum -a 256 $(DIST_DIR)/refloom-$(VERSION)-darwin-arm64.zip | cut -d' ' -f1)"

setup-skills:
	bash scripts/setup-agent-skills.sh
