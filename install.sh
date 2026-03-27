#!/bin/bash

# install.sh - One-line installer for ollama-symlinks
# Usage: curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/install.sh | bash

# Repository information
REPO_OWNER="qaribhaider"
REPO_NAME="ollama-to-lmstudio-symlinks"
BINARY_NAME="ollama-symlinks"
INSTALL_DIR="/usr/local/bin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to log messages
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Detect OS and Architecture
detect_system() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    case "$OS" in
        linux) OS="linux" ;;
        darwin) OS="macos" ;;
        *) log_error "Unsupported OS: $OS"; exit 1 ;;
    esac

    echo "${OS}-${ARCH}"
}

# Fetch latest version from GitHub API
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        VERSION=$(curl -s "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi

    if [ -z "$VERSION" ]; then
        # Fallback to a default if API fails (could also be hardcoded or fetched from elsewhere)
        VERSION="v0.2.1" 
    fi
    echo "$VERSION"
}

# Main installation logic
install() {
    local SYSTEM_INFO=$(detect_system)
    local VERSION=$(get_latest_version)
    local DOWNLOAD_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${BINARY_NAME}-${SYSTEM_INFO}"

    if [ "$OS" = "windows" ]; then
        DOWNLOAD_URL="${DOWNLOAD_URL}.exe"
    fi

    log_info "Detected system: ${SYSTEM_INFO}"
    log_info "Latest version: ${VERSION}"
    log_info "Downloading from: ${DOWNLOAD_URL}"

    if [ "$DRY_RUN" = "true" ]; then
        log_info "DRY RUN: Would download and install to ${INSTALL_DIR}/${BINARY_NAME}"
        return 0
    fi

    # Create a temporary directory
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    # Download binary
    if ! curl -L -o "${TMP_DIR}/${BINARY_NAME}" "$DOWNLOAD_URL"; then
        log_error "Failed to download binary from $DOWNLOAD_URL"
        exit 1
    fi

    # Make executable
    chmod +x "${TMP_DIR}/${BINARY_NAME}"

    # Move to install directory
    log_info "Installing to ${INSTALL_DIR} (may require sudo)..."
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    log_success "${BINARY_NAME} installed successfully to ${INSTALL_DIR}"
    log_info "You can now run it using: ${BINARY_NAME} --help"
}

# Check for flags
DRY_RUN=false
for arg in "$@"; do
    if [ "$arg" = "--dry-run" ]; then
        DRY_RUN=true
    fi
done

# Execute if not being sourced (for testing)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    install
fi
