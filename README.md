# Ollama-LMStudio Symlink Utility

A modular Go utility to create symbolic links between Ollama and LM Studio, allowing you to share model files bidirectional without duplicating storage space.

## 🎯 Purpose

If you have models downloaded in Ollama and want to use them in LM Studio, you typically need to download them again, wasting disk space. This tool creates symbolic links so both applications can share the same model files.

## ✨ Features

- **🔄 Bidirectional Sync**: Link Ollama models to LM Studio OR LM Studio models to Ollama
- **🔍 Dynamic Discovery**: Automatically scans and discovers models in both applications
- **🧹 Interactive Cleanup**: Safely remove symlinks through the interactive `delete` command
- **⚙️ Configurable Paths**: Customize directories via command-line flags
- **🛡️ Safe Operations**: Never overwrites existing files; ignores existing symlinks to avoid circular loops
- **📊 Clear Status**: Shows what was created vs. skipped
- **🧪 Dry Run**: Preview changes without making them
- **🎯 Multi-component Support**: Handles complex models (e.g., LLaVA with projector files)

## 🚀 Installation

### One-Line Quick Install
Install the latest version automatically on macOS or Linux (Follow manual installation for Windows):

```bash
curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/install.sh | bash
```

### Manual Installation
You can also download the pre-compiled binaries from the [Releases](https://github.com/qaribhaider/ollama-to-lmstudio-symlinks/releases) page.

1. Download the binary for your OS and architecture.
2. Make it executable: `chmod +x ollama-symlinks-*`.
3. Move it to your path: `sudo mv ollama-symlinks-* /usr/local/bin/ollama-symlinks`.

## 🚀 Installation

### One-Line Quick Install
Install the latest version automatically on macOS or Linux (Follow manual installation for Windows):

```bash
curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/install.sh | bash
```

### Manual Installation
You can also download the pre-compiled binaries from the [Releases](https://github.com/qaribhaider/ollama-to-lmstudio-symlinks/releases) page.

1. Download the binary for your OS and architecture.
2. Make it executable: `chmod +x ollama-symlinks-*`.
3. Move it to your path: `sudo mv ollama-symlinks-* /usr/local/bin/ollama-symlinks`.

## 🗑️ Uninstallation

To remove the utility from your system:

```bash
sudo curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/uninstall.sh | bash
```

## 🚀 Quick Start

### Run with Defaults

```bash
./ollama-symlinks
```

This will:

- Scan `~/.ollama/models` for Ollama models
- Create symlinks in `~/.cache/lm-studio/models/ollama/`

⚠️ **Important**: Do not delete symlinked models through the LM Studio UI, as it may cause issues if you try to symlink them again. Instead, use the built-in cleanup tool: `ollama-symlinks delete --from lmstudio`.

## 📖 Usage

### Basic Usage

#### 1. Link Ollama → LM Studio (Forward)
This scans your Ollama manifests and creates symlinks in LM Studio under the `ollama` provider.

```bash
# Normal execution
./ollama-symlinks

# Dry run (preview changes)
./ollama-symlinks --dry-run
```

#### 2. Link LM Studio → Ollama (Reverse)
This scans GGUF files in LM Studio and registers them with Ollama using a prefix (default `lms-`).

```bash
# Normal execution
./ollama-symlinks --reverse

# With custom prefix and dry run
./ollama-symlinks --reverse --name-prefix="my-model" --dry-run
```

#### 3. Interactively Delete Symlinks
Safely remove symlinks created by this tool without touching the original model files.

```bash
# Remove from LM Studio (the 'ollama' folder)
./ollama-symlinks delete --from lmstudio

# Remove from Ollama (the 'lms-' prefixed models)
./ollama-symlinks delete --from ollama
```

### ⚙️ Command Line Arguments

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--ollama-dir` | `string` | `~/.ollama/models` | Path to the root Ollama models directory. |
| `--lmstudio-dir` | `string` | `~/.cache/lm-studio/models` | Path to the root LM Studio models directory. |
| `--reverse` | `bool` | `false` | Enable **Reverse Mode**: Link models from LM Studio → Ollama. |
| `--name-prefix` | `string` | `lms` | Prefix used for naming models when importing into Ollama. |
| `--skip-provider` | `string` | `ollama` | Folder name in LM Studio where symlinks are created. |
| `--dry-run` | `bool` | `false` | Show logs of what would happen without making changes. |
| `--verbose` | `bool` | `false` | Enable detailed logging of the process. |
| `--version` | `bool` | `false` | Display the current version of the utility. |
| `--help` | `bool` | `false` | Show the help message with all available flags. |

#### `delete` Subcommand Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--from` | `string` | *(Required)* | Target for deletion: `ollama` or `lmstudio`. |
| `--dry-run` | `bool` | `false` | Preview which symlinks would be removed. |
| `--verbose` | `bool` | `false` | Show detailed paths during the deletion. |

```bash
# Using custom paths for both applications
./ollama-symlinks \
  --ollama-dir="/Volumes/External/Ollama" \
  --lmstudio-dir="/Volumes/External/LMStudio" \
  --verbose
```

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
echo "v0.2.0" > VERSION
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

## 💡 Example Output

```
🔍 Scanning Ollama models in: /Users/user/.ollama/models
🎯 Target LM Studio directory: /Users/user/.cache/lm-studio/models

📦 Found 4 models:
  • llama3.2-3b
  • gemma3-4b
  • qwen3-8b
  • llava-7b

🔗 CREATING: llama3.2-3b
🔗 CREATING: gemma3-4b
⏭️  SKIPPED: qwen3-8b (already exists)
🔗 CREATING: llava-7b

✅ Summary: 3 created, 1 skipped
🎉 Models are now available in LM Studio under the 'ollama' provider
```

## 🛠️ Requirements

### To Build

- Go 1.26.1+ (Modular Go project)

### To Run

- **No Go required** - the compiled binary is standalone
- macOS, Linux, or Windows
- Existing Ollama installation with downloaded models
- LM Studio installation

## ⚠️ Important Notes

- **Symbolic Links**: The tool creates symlinks, not copies. If you delete Ollama models, LM Studio won't be able to access them
- **Cross-Platform**: Works on macOS, Linux, and Windows (Windows requires appropriate permissions for symlinks)
- **Safe Operation**: Never overwrites existing files or symlinks
- **Storage Savings**: Can save 10-50GB+ depending on your model collection
- **Deletion in LM Studio**: Do not delete symlinked models through the LM Studio UI. Instead, use the built-in cleanup tool: `ollama-symlinks delete --from lmstudio`.

## 🔍 Troubleshooting

### "Permission denied" on Windows

Run the command prompt as Administrator when creating symlinks on Windows.

### Models not appearing in LM Studio

1. Restart LM Studio after creating symlinks
2. Check that the symlinks point to valid files: `ls -la ~/.cache/lm-studio/models/ollama/*/`

### "No models found"

Verify your Ollama directory contains models:

```bash
ls -la ~/.ollama/models/manifests/
```

## 🤝 Contributing

Feel free to submit issues or pull requests to improve this tool!

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
