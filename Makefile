.PHONY: build test clean python-setup lint dist

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
	CGO_ENABLED=$(CGO_ENABLED) go vet -tags "$(GO_TAGS)" ./...

dist: build
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)/refloom/python/refloom_worker
	cp $(BUILD_DIR)/$(BINARY) $(DIST_DIR)/refloom/
	cp -r python/refloom_worker/*.py $(DIST_DIR)/refloom/python/refloom_worker/
	cp python/refloom_worker/requirements.txt $(DIST_DIR)/refloom/python/refloom_worker/
	cp config/refloom.example.yaml $(DIST_DIR)/refloom/
	cd $(DIST_DIR) && zip -r refloom-$(VERSION)-darwin-arm64.zip refloom/
	@echo "Distribution created: $(DIST_DIR)/refloom-$(VERSION)-darwin-arm64.zip"
