# Ollama-LMStudio Symlink Utility

A modular Go utility to create symbolic links between Ollama and LM Studio, allowing you to share model files bidirectional without duplicating storage space.

## 🎯 Purpose

If you have models downloaded in Ollama and want to use them in LM Studio, you typically need to download them again, wasting disk space. This tool creates symbolic links so both applications can share the same model files.

## ✨ Features

- **🔄 Bidirectional Sync**: Link Ollama models to LM Studio OR LM Studio models to Ollama
- **🔍 Dynamic Discovery**: Automatically scans and discovers models in both applications
- **🧹 Interactive & Auto Cleanup**: Safely remove symlinks through the interactive `delete` command, or auto-discover and wipe broken ghost-links using `cleanup`
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

## 🗑️ Uninstallation

To remove the utility from your system:

```bash
curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/uninstall.sh | sudo bash
```
## 🚀 Quick Start

### Run with Defaults

```bash
./ollama-symlinks
```

This will:

- Scan `~/.ollama/models` for Ollama models
- Create symlinks in `~/.cache/lm-studio/models/ollama/`

⚠️ **Important**: Do not delete symlinked models through the LM Studio UI, as it may cause issues if you try to symlink them again. Instead, use the built-in tool: `ollama-symlinks delete --from lmstudio`.

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

#### 4. Auto-discover and Cleanup Broken Symlinks
If you delete an Ollama model directly using `ollama rm`, the symlinks in LM Studio may become broken. The `cleanup` command scans and helps you safely remove these broken ghost-links.

```bash
# Preview broken symlinks without deleting
./ollama-symlinks cleanup --dry-run

# Interactively remove broken symlinks
./ollama-symlinks cleanup
```

### ⚙️ Command Line Arguments

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--ollama-dir` | `string` | `~/.ollama/models` | Path to the root Ollama models directory. |
| `--lmstudio-dir` | `string` | `~/.cache/lm-studio/models` | Path to the root LM Studio models directory. |
| `--reverse` | `bool` | `false` | Enable **Reverse Mode**: Link models from LM Studio → Ollama. |
| `--name-prefix` | `string` | `lms` | Prefix used for naming models when importing into Ollama. |
| `--skip-provider` | `string` | `ollama` | Folder name in LM Studio where symlinks are created. |
| `--hardlinks` | `bool` | `false` | Use hard links instead of symlinks. Fixes "0 bytes" or "failed to load" issues on Windows. |
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

#### `cleanup` Subcommand Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--dry-run` | `bool` | `false` | Preview which broken symlinks would be removed. |
| `--verbose` | `bool` | `false` | Enable detailed logging of the process. |

```bash
# Using custom paths for both applications
./ollama-symlinks \
  --ollama-dir="/Volumes/External/Ollama" \
  --lmstudio-dir="/Volumes/External/LMStudio" \
  --verbose
```


## ⚠️ Important Notes

- **Symbolic Links**: The tool creates symlinks, not copies. If you delete Ollama models, LM Studio won't be able to access them
- **Cross-Platform**: Works on macOS, Linux, and Windows (Windows requires appropriate permissions for symlinks)
- **Safe Operation**: Never overwrites existing files or symlinks
- **Storage Savings**: Can save 10-50GB+ depending on your model collection
- **Deletion in LM Studio**: Do not delete symlinked models through the LM Studio UI. Instead, use the built-in cleanup tool: `ollama-symlinks delete --from lmstudio`.

## 🔍 Troubleshooting

### "Permission denied" on Windows

Run the command prompt as Administrator when creating symlinks on Windows.

### "Failed to load model" or models showing as "0 bytes" in LM Studio (Windows)

Recent versions of LM Studio or strict Windows configurations may block standard symbolic links from loading. To fix this, switch to hard links:

1. **First, delete the broken/0-byte symlinks:**
```bash
./ollama-symlinks delete --from lmstudio
```

2. **Then, re-run the tool using the `--hardlinks` flag:**
```bash
./ollama-symlinks --hardlinks
```

**⚠️ Important Note on Space:** Hard links are identical to regular files. If you run `ollama rm <model>` to clear disk space, the space **won't actually be freed** until you also delete the linked file from LM Studio using `ollama-symlinks delete --from lmstudio`.

### Models not appearing in LM Studio

1. Restart LM Studio after creating symlinks
2. Check that the symlinks point to valid files: `ls -la ~/.cache/lm-studio/models/ollama/*/`

### "No models found"

Verify your Ollama directory contains models:

```bash
ls -la ~/.ollama/models/manifests/
```

## 🤝 Contributing

Feel free to submit issues or pull requests to improve this tool! For details on building from source, system requirements, and an architecture overview, please see our [Contributing Guide](CONTRIBUTING.md).

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
