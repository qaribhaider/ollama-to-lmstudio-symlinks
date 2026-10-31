---
name: release
description: Prepare a new release by verifying the build and bumping VERSION. Merging the bump to main publishes the release.
disable-model-invocation: true
argument-hint: "[version, e.g. v0.3.12]"
---

Releases are published by `.github/workflows/build.yml` on every push to `main`, tagged from the `VERSION` file. So a release is a `VERSION` bump that gets merged. Do not tag or publish by hand.

1. Refuse to continue if the working tree has uncommitted changes. If on `main`, tell the user to switch to a branch first.
2. Run `go test ./...` and `golangci-lint run ./...`. Stop on test failures. Lint findings in code you did not touch are pre-existing, so mention them but do not block.
3. Read `VERSION` and the latest tag (`git describe --tags --abbrev=0`). Use the version from `$ARGUMENTS`; if none was given, propose a patch bump (`v0.3.11` becomes `v0.3.12`) and confirm before writing.
4. Write the new version to `VERSION` (the `v` prefix is part of the value).
5. Summarise `git log <last-tag>..HEAD --oneline` as release notes the user can paste into the PR description.
6. Stop there. Commit, push, and PR only if the user asks.
