.PHONY: build clean test version

# Default target
all: build

# Build the binary
build:
	@echo "Building ollama-symlinks..."
	@./build.sh

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
