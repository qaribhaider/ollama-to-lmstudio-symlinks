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
echo "v0.3.0" > VERSION
```

After updating the version, commit the changes and push to trigger a new release.

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
