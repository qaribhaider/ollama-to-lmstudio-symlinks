#!/bin/bash

# install.sh - One-line installer for ollama-symlinks
# Usage: curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/install.sh | bash

# Repository information
REPO_OWNER="qaribhaider"
REPO_NAME="ollama-to-lmstudio-symlinks"
BINARY_NAME="ollama-symlinks"
INSTALL_DIR="/usr/local/bin"

# Release requested by the caller (VERSION=vX.Y.Z). Captured here because
# install_binary declares its own local VERSION, which would hide the variable.
REQUESTED_VERSION="${VERSION:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to log messages
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
# Errors go to stderr: they are often raised inside $(...), where stdout is captured.
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

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

# Fetch latest version from GitHub API. Set VERSION=vX.Y.Z to install a specific
# release, which also works when the API is rate limited.
get_latest_version() {
    VERSION="$REQUESTED_VERSION"

    if [ -z "$VERSION" ] && command -v curl >/dev/null 2>&1; then
        VERSION=$(curl -fsS --proto '=https' --tlsv1.2 "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi

    if [ -z "$VERSION" ]; then
        log_error "Could not determine the latest version (GitHub API unavailable or rate limited). Re-run with VERSION=vX.Y.Z to choose one."
        exit 1
    fi

    # Validate version string to prevent URL injection
    if ! echo "$VERSION" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
        log_error "Invalid version string received: $VERSION"
        exit 1
    fi

    echo "$VERSION"
}

# Main installation logic
install_binary() {
    # Assign separately from `local` so a failure in the command is not masked,
    # and stop here because exit inside $(...) only ends the subshell.
    local SYSTEM_INFO VERSION
    SYSTEM_INFO=$(detect_system) || exit 1
    VERSION=$(get_latest_version) || exit 1
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
    if ! curl -fL --proto '=https' --tlsv1.2 -o "${TMP_DIR}/${BINARY_NAME}" "$DOWNLOAD_URL"; then
        log_error "Failed to download binary from $DOWNLOAD_URL"
        exit 1
    fi

    # Fetch and verify checksum
    local CHECKSUM_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/checksums.txt"
    log_info "Verifying binary checksum..."
    local EXPECTED_CHECKSUM
    EXPECTED_CHECKSUM=$(curl -fsSL --proto '=https' --tlsv1.2 "${CHECKSUM_URL}" | grep "${BINARY_NAME}-${SYSTEM_INFO}" | awk '{print $1}')
    
    if [ -z "$EXPECTED_CHECKSUM" ]; then
        log_error "Could not find expected checksum for ${SYSTEM_INFO} in checksums.txt"
        exit 1
    fi

    local ACTUAL_CHECKSUM
    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL_CHECKSUM=$(sha256sum "${TMP_DIR}/${BINARY_NAME}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        ACTUAL_CHECKSUM=$(shasum -a 256 "${TMP_DIR}/${BINARY_NAME}" | awk '{print $1}')
    else
        log_error "Neither sha256sum nor shasum found to verify binary integrity."
        exit 1
    fi

    if [ "$EXPECTED_CHECKSUM" != "$ACTUAL_CHECKSUM" ]; then
        log_error "Checksum mismatch! Binary may be corrupted or tampered with."
        exit 1
    fi
    log_info "Checksum verified successfully."

    # Copy into place with the right mode. mv from /tmp is a cross-filesystem
    # copy, so an interrupted run could leave a partial binary on the PATH.
    log_info "Installing to ${INSTALL_DIR} (may require sudo)..."
    if [ -w "$INSTALL_DIR" ]; then
        install -m 0755 "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}" || exit 1
    else
        sudo install -m 0755 "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}" || exit 1
    fi

    log_success "${BINARY_NAME} installed successfully to ${INSTALL_DIR}"
    
    # Check if INSTALL_DIR is in PATH
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        log_warn "${INSTALL_DIR} is not in your PATH. You may need to add it to your shell profile."
    fi

    log_info "If the command is not found, run 'rehash' (zsh) or 'hash -r' (bash) to refresh your shell."
    log_info "You can now run it using: ${BINARY_NAME} --help"
}

# Check for flags
DRY_RUN=false
for arg in "$@"; do
    if [ "$arg" = "--dry-run" ]; then
        DRY_RUN=true
    fi
done

# Execute unless skipped (for testing)
if [ -z "$SKIP_INSTALL" ]; then
    install_binary
fi
