#!/bin/bash

# test_install.sh - Mock testing for install.sh
# Usage: ./test_install.sh

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# Mocking variables
MOCK_OS=""
MOCK_ARCH=""

# Define uname to return our mocks
uname() {
    if [ "$1" = "-s" ]; then echo "$MOCK_OS"; fi
    if [ "$1" = "-m" ]; then echo "$MOCK_ARCH"; fi
}
export -f uname

# Redefine log functions for silence/testing
log_info() { :; }
log_success() { :; }
log_error() { :; }

# Source install.sh but don't let it run (it checks BASH_SOURCE)
source ./install.sh

# Test results
total_tests=0
passed_tests=0

run_test() {
    local name="$1"
    local os="$2"
    local arch="$3"
    local expected="$4"
    
    ((total_tests++))
    
    MOCK_OS="$os"
    MOCK_ARCH="$arch"
    
    actual=$(detect_system)
    
    if [ "$actual" = "$expected" ]; then
        echo -e "${GREEN}✅ PASSED: $name${NC} (Expected $expected, Got $actual)"
        ((passed_tests++))
    else
        echo -e "${RED}❌ FAILED: $name${NC} (Expected $expected, Got $actual)"
    fi
}

echo "Running install.sh logic tests..."
echo "-----------------------------------"

run_test "macOS Intel" "Darwin" "x86_64" "macos-amd64"
run_test "macOS Apple Silicon" "Darwin" "arm64" "macos-arm64"
run_test "Linux Intel" "Linux" "x86_64" "linux-amd64"
run_test "Linux ARM" "Linux" "aarch64" "linux-arm64"

# Additional check for Windows (though the script doesn't explicitly support it for curl|bash, the logic could)
# run_test "Windows" "Windows_NT" "x86_64" "windows-amd64"

echo "-----------------------------------"
echo "Result: $passed_tests / $total_tests tests passed."

if [ $passed_tests -eq $total_tests ]; then
    echo "🎉 ALL TESTS PASSED!"
    exit 0
else
    echo "🚨 SOME TESTS FAILED!"
    exit 1
fi
