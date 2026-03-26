#!/bin/bash

# Build script for ollama-symlinks
# This script reads the version from VERSION file and builds the binary

set -e

# Read version from VERSION file
VERSION=$(cat VERSION)
echo "Building version: $VERSION"

# Build the binary
echo "Building binary..."
go build -ldflags="-X 'main.Version=$VERSION'" -o ollama-symlinks ./cmd/ollama-symlinks

echo "Build complete: ollama-symlinks version $VERSION"
