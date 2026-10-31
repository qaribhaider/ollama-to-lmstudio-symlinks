#!/usr/bin/env bash
# Tests check-version.sh. Run from the repository root:
#   .github/scripts/check-version_test.sh
set -uo pipefail

script="$(dirname "$0")/check-version.sh"
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

failures=0

# expect <pass|fail> <description> <file contents, printf format> <latest tag>
expect() {
  local want=$1 name=$2 contents=$3 latest=$4
  printf '%b' "$contents" > "$dir/VERSION"
  if "$script" "$dir/VERSION" "$latest" >/dev/null 2>&1; then got=pass; else got=fail; fi
  if [[ $got == "$want" ]]; then
    echo "ok    $name"
  else
    echo "FAIL  $name (expected $want, got $got)"
    failures=$((failures + 1))
  fi
}

expect pass "newer patch"                'v0.3.12\n' v0.3.11
expect pass "newer minor"                'v0.4.0\n'  v0.3.11
expect pass "10 is higher than 9"        'v0.3.10\n' v0.3.9
expect pass "no tags yet"                'v0.1.0\n'  ''
expect pass "no trailing newline"        'v0.4.0'    v0.3.11
expect fail "same as latest"             'v0.3.11\n' v0.3.11
expect fail "lower than latest"          'v0.3.9\n'  v0.3.11
expect fail "missing v prefix"           '0.4.0\n'   v0.3.11
expect fail "trailing space"             'v0.4.0 \n' v0.3.11
expect fail "windows line ending"        'v0.4.0\r\n' v0.3.11
expect fail "pre-release suffix"         'v0.4.0-rc1\n' v0.3.11
expect fail "two lines"                  'v0.4.0\nv0.4.1\n' v0.3.11
expect fail "empty file"                 ''          v0.3.11

if ((failures > 0)); then
  echo "$failures check(s) failed"
  exit 1
fi
echo "all checks passed"
