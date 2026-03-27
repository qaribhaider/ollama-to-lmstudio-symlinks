#!/bin/bash

# uninstall.sh - One-line uninstaller for ollama-symlinks
# Usage: curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/uninstall.sh | bash

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

uninstall() {
    local target="${INSTALL_DIR}/${BINARY_NAME}"

    if [ ! -f "$target" ]; then
        log_error "${BINARY_NAME} is not installed in ${INSTALL_DIR}"
        exit 1
    fi

    log_info "Removing ${target} (may require sudo)..."
    if [ -w "$target" ]; then
        rm "$target"
    else
        sudo rm "$target"
    fi

    if [ $? -eq 0 ]; then
        log_success "${BINARY_NAME} uninstalled successfully"
    else
        log_error "Failed to remove ${BINARY_NAME}"
        exit 1
    fi
}

uninstall
