.PHONY: build test clean python-setup lint

BINARY := refloom
BUILD_DIR := bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/refloom

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)

python-setup:
	cd python/refloom_worker && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt

lint:
	go vet ./...
