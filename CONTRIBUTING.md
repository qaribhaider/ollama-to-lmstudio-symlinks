# Contributing to Ollama-LMStudio Symlink Utility

First off, thank you for considering contributing to this utility!

## 🚀 Fancy some changes?

### Build from Source

#### Using Make (Recommended)

```bash
# Build the binary
make build

# Clean built files
make clean

# Run tests
make test

# Smoke tests against a real Ollama (manual, not run in CI)
make smoke

# Show version
make version

# Install to /usr/local/bin
make install
```

#### Using the Build Script

```bash
# Build the binary
./build.sh

# Clean built files
rm -f ollama-symlinks
```

#### Using Go Directly

```bash
# Build the binary
go build -o ollama-symlinks ./cmd/ollama-symlinks

# Run the binary
./ollama-symlinks
```

### Version Management

The version is stored in the `VERSION` file. To update the version:

```bash
# Using Make (recommended)
make update-version

# Or manually
echo "v0.3.1" > VERSION
```

### Releasing

A release is published when a change to `VERSION` is merged into `main`. Nothing else triggers one.

1. Run `make smoke` if the change touches linking, discovery, delete or cleanup.
2. Bump `VERSION` (`vX.Y.Z`, higher than the latest release) in the PR. A PR that changes `VERSION` also runs the **Release check** workflow: it validates the version and runs the tests on Linux, macOS and Windows before you can merge.
3. Merge. The **Build and Release** workflow builds all binaries, writes `checksums.txt`, attests their provenance, and publishes the release with generated notes.

Verify a downloaded binary was built by this repository's workflow:

```bash
gh attestation verify ollama-symlinks-macos-arm64 --repo qaribhaider/ollama-to-lmstudio-symlinks
```

**If a release is bad:** the workflow refuses to publish a version whose tag already exists, so you cannot overwrite it. Delete the release on GitHub (the installer uses the latest release, so it falls back to the previous one), fix the problem, and publish the next patch version.

## 🔧 How It Works

### Forward Mode (Ollama → LM Studio)
1. **Scans** the Ollama manifests directory (`~/.ollama/models/manifests/`)
2. **Parses** JSON manifest files to identify model components and blobs.
3. **Maps** names to an LM Studio-friendly format (e.g., `llama3-7b-latest.gguf`).
4. **Creates** an `ollama` provider directory in your LM Studio models folder.
5. **Generates** symbolic links pointing to the original Ollama blobs.

### Reverse Mode (LM Studio → Ollama)
1. **Scans** the LM Studio models directory for `.gguf` files.
2. **Filters** out existing symlinks and the `ollama` provider folder to avoid circular loops.
3. **Calculates** SHA256 checksums to create Ollama-compatible blob identifiers.
4. **Symlinks** the GGUF file into the Ollama `blobs` directory.
5. **Registers** the model with Ollama using `ollama create` and a custom Modelfile.

## 🛠️ Requirements

### To Build

- Go 1.26.1+ (Modular Go project)

### To Run

- **No Go required** - the compiled binary is standalone
- macOS, Linux, or Windows
- Existing Ollama installation with downloaded models
- LM Studio installation
