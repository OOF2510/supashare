.PHONY: build clean

# Variables
VERSION ?=
PROJECT_NAME := $(shell basename $(CURDIR))
DIST_DIR := dist
BINARY_NAME := $(PROJECT_NAME)-$(VERSION).x86_64
BINARY_PATH := $(DIST_DIR)/$(VERSION)/$(BINARY_NAME)

# Default target
all: build

# Build target
build:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make build VERSION=version"; \
		exit 1; \
	fi
	@echo "=== Running gofmt, go mod download and go mod tidy... ==="
	@gofmt -w .
	@go mod download
	@go mod tidy
	@echo "=== Building the project... ==="
	@mkdir -p $(DIST_DIR)/$(VERSION)
	@go build -v -ldflags="-s -w" -trimpath -o $(BINARY_PATH) .
	@echo "=== Compressing binary with UPX... ==="
	@if command -v upx >/dev/null 2>&1; then \
		upx --best --lzma $(BINARY_PATH); \
	else \
		echo "UPX not found, skipping compression"; \
	fi
	@echo "=== Build finished. Setting executable permissions... ==="
	@chmod +x $(BINARY_PATH)
	@echo "=== Copying pages... ==="
	@cp -rv pages/ $(DIST_DIR)/$(VERSION)/pages/
	@if [ -f .env ]; then \
		echo "=== Linking .env file... ==="; \
		ln -sf $(CURDIR)/.env $(CURDIR)/$(DIST_DIR)/$(VERSION)/.env; \
	fi
	@echo "=== Binary size: ==="
	@ls -lh $(BINARY_PATH)
	@echo "=== Build completed ==="

# Clean target
clean:
	@rm -fr $(DIST_DIR)
