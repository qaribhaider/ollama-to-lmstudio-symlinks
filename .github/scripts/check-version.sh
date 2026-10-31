#!/usr/bin/env bash
# Usage: check-version.sh <VERSION file> [latest-tag]
#
# Fails unless the VERSION file holds a clean vX.Y.Z that is higher than the
# latest released tag. The release workflow reads the file with `cat` and uses
# it as the tag name, so stray whitespace or a repeated version would publish a
# broken or conflicting release.
set -euo pipefail

file=${1:?usage: check-version.sh <VERSION file> [latest-tag]}
latest=${2:-}

contents=$(cat "$file")

# Pre-releases are rejected on purpose: `sort -V` orders v1.0.0-rc1 after v1.0.0.
if [[ ! $contents =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error file=$file::VERSION must be exactly vX.Y.Z (no spaces, no suffix), got: '$contents'"
  exit 1
fi

if [[ -n $latest ]]; then
  if [[ $contents == "$latest" ]]; then
    echo "::error file=$file::$contents is already released; bump VERSION to publish a new release"
    exit 1
  fi
  newest=$(printf '%s\n%s\n' "$latest" "$contents" | sort -V | tail -n 1)
  if [[ $newest != "$contents" ]]; then
    echo "::error file=$file::$contents is lower than the latest release $latest"
    exit 1
  fi
fi

echo "VERSION $contents is valid${latest:+ and newer than $latest}"
