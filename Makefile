.PHONY: build test clean python-setup lint

BINARY := refloom
BUILD_DIR := bin
GO_TAGS := fts5
CGO_ENABLED := 1

build:
	CGO_ENABLED=$(CGO_ENABLED) go build -tags "$(GO_TAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/refloom

test:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags "$(GO_TAGS)" ./...

clean:
	rm -rf $(BUILD_DIR)

python-setup:
	cd python/refloom_worker && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt

lint:
	CGO_ENABLED=$(CGO_ENABLED) go vet -tags "$(GO_TAGS)" ./...
