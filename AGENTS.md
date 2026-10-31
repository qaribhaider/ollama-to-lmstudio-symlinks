# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Commands

```bash
make build                                    # go build -o ollama-symlinks ./cmd/ollama-symlinks
make test                                     # go test -v ./...
go test ./internal/linking -run TestName -v   # single test
make smoke                                    # manual, needs ollama installed; not in CI
```

`make smoke` runs the flows against a real Ollama using its own server and temp store (the first run downloads a small model into the user cache). Run it after changing linking, discovery or delete. Unit tests never execute a real `ollama`: a `TestMain` stubs the lookup, so keep any new test that shells out behind that stub.

CI (`.github/workflows/ci.yml`) runs `go test -v ./...` and a build with `-ldflags="-X 'main.Version=$(cat VERSION)'"`. The version lives in the `VERSION` file and is injected at build time, so a plain `go build` reports a default version. A release is published only when a push to `main` changes `VERSION` (`make update-version`): `build.yml` is path-filtered to that file and fails if the tag already exists.

## Architecture

A single-binary Go CLI (Bubble Tea / huh for the interactive UI) that shares model files between Ollama and LM Studio by linking instead of copying. Two directions:

- **Forward (Ollama → LM Studio):** parse Ollama manifests in `<ollama>/models/manifests`, then link the referenced blobs into `<lmstudio>/<skip-provider>/...` under readable `.gguf` names.
- **Reverse (`--reverse`, LM Studio → Ollama):** find `.gguf` files, SHA256 them, link into Ollama's `blobs/` as `sha256-<hash>`, then register via `ollama create` with a generated Modelfile.

Layout:

- `cmd/ollama-symlinks` is the command layer, one file per command (`forward.go`, `reverse.go`, `delete.go`, `cleanup.go`, `status.go`). `main.go` holds `runApp`, which binds flags into an `options` struct and dispatches to those commands or the interactive `runMainMenu`; subcommands get a copy of `options`. `select.go` and `prompt.go` hold the shared selection, results and confirm helpers. The `stdin` parameter is threaded through but the `huh` prompts read the real terminal, so interactive paths cannot be driven from tests; keep decisions in pure functions (`findOwnedModels`, `confirmOutcome`) and test those.
- `internal/ollama` and `internal/lmstudio` discover models and locate/validate the respective executables (`executable.go`, skipped with `--skip-checks`). They return the types in `internal/models`.
- `internal/linking` does the actual file work: `ProcessModel` (forward), `ProcessLMStudioModel` (reverse), `linkBlob`, plus listing, removing, and broken-link detection. `status.go` computes the savings numbers. `hardlink_unix.go` and `hardlink_windows.go` are build-tagged and differ in how hard-link identity is detected.
- `internal/gguf` reads GGUF headers. `internal/ui` holds shared lipgloss styles.

## Things that are easy to get wrong

- Sharded models (`...-00001-of-0000N.gguf`) and vision projectors (`mmproj-*.gguf`) must be grouped and linked together with their main model. Both have had dedicated bugfixes, so check the existing tests before changing the grouping logic.
- Names derived from model paths go through `SecureJoin` and `SanitizeModelName` in `internal/linking`. Route any new path built from manifest or user-supplied names through them.
- The tool must never overwrite an existing file and must only delete links, never the original weights. Anything that lists or removes files must go through `linking.IsManagedLink` (a symlink, or a file with more than one hard link); a plain file is original data. Reverse mode skips symlinks and the provider folder to avoid circular links.
- `--skip-provider` is one value shared by forward, reverse, status, delete and cleanup, validated by `validateProviderName`. delete and cleanup act on everything inside that folder, so never accept an empty or path-like value.
- `--hardlinks` never falls back to symlinks: a failed hard link is an error.
- Ollama keeps manifests in two places: `manifests/registry.ollama.ai/...` for pulled models and, on newer versions, `manifests-v2/ollama.com/...` for models created locally (which includes everything reverse mode registers). `DiscoverModels` must read both; the unit tests use fixtures, so only `make smoke` shows when Ollama changes its layout again.
- `--hardlinks` exists because Windows symlinks need Developer Mode or admin rights and can show up as 0-byte files. See `TROUBLESHOOTING.md`.
- `--dry-run` has to be honoured by every function that touches the filesystem; it is passed down explicitly rather than being global state.
