.PHONY: all build clean test smoke version install update-version

# update-version uses bash's `read -p`
SHELL := /bin/bash

VERSION := $(shell cat VERSION)

# Default target
all: build

# Build the binary, injecting the version the same way CI does
build:
	@echo "Building ollama-symlinks $(VERSION)..."
	@go build -ldflags="-X 'main.Version=$(VERSION)'" -o ollama-symlinks ./cmd/ollama-symlinks

# Clean built files
clean:
	@echo "Cleaning built files..."
	@rm -f ollama-symlinks
	@rm -f ollama-symlinks-*
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Manual smoke tests against a real Ollama (not part of CI). They use their own
# server and temp model store; the first run downloads a small model into the
# user cache. Override the model with SMOKE_MODEL=name:tag.
smoke:
	@echo "Running smoke tests (needs ollama installed)..."
	@go test -tags smoke -run Smoke -count=1 -v -timeout 20m ./cmd/ollama-symlinks

# Show version
version:
	@./ollama-symlinks --version

# Install the binary to /usr/local/bin
install: build
	@echo "Installing ollama-symlinks to /usr/local/bin..."
	@sudo cp ollama-symlinks /usr/local/bin/
	@echo "Installation complete"

# Update version
update-version:
	@read -p "Enter new version (current: $(shell cat VERSION)): " new_version && \
	echo "$$new_version" > VERSION && \
	echo "Version updated to $$new_version"
