# Ollama ↔ LM Studio Symlink Utility

A fast, cross-platform Go CLI to create symbolic links between **Ollama** and **LM Studio**, sharing models bidirectionally without duplicating gigabytes of storage space.

---

## ✨ Features

- 🔄 **Bidirectional Linking**: Link Ollama models to LM Studio OR LM Studio models to Ollama.
- 🧩 **Sharded GGUF Support**: Automatically groups and links multi-file models (`00001-of-0000N.gguf`).
- 👁️ **Vision & Multimodal Adapters**: Pairs and links multimodal projector adapters (`mmproj-*.gguf`).
- 🌐 **Registries & Namespaces**: Full support for Hugging Face (`hf.co/...`), custom namespaces, and standard models.
- 📊 **Storage Analytics (`status`)**: Measures active links, detects broken symlinks, and calculates deduplicated disk savings.
- 🧹 **Dual-Directory Cleanup (`cleanup`)**: Scans both LM Studio and Ollama blobs to find and wipe broken ghost-links.
- 🛡️ **Safe & Interactive (`delete`)**: Selectively remove links without touching original weights. Never overwrites existing files.
- 💻 **Cross-Platform**: Works natively on macOS, Linux, and Windows (with `--hardlinks` fallback).

---

## 📥 Installation

### Quick Install (macOS & Linux)
```bash
curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/install.sh | bash
```

### Manual Download
Download pre-compiled binaries for macOS, Linux, or Windows from the [Releases](https://github.com/qaribhaider/ollama-to-lmstudio-symlinks/releases) page:
```bash
chmod +x ollama-symlinks-*
sudo mv ollama-symlinks-* /usr/local/bin/ollama-symlinks
```

---

## 🚀 Quick Start

Run the interactive terminal menu:

```bash
ollama-symlinks
```

The interactive UI allows you to view storage savings, link models forward or reverse, or clean up links with arrow-key navigation.

---

## 📖 Command Cheatsheet

### 1. View Status & Storage Savings
Inspect active links, broken symlinks, and exact disk space saved across both applications:
```bash
ollama-symlinks status
# Add --verbose to list individual files and targets
ollama-symlinks status --verbose
```

### 2. Link Ollama → LM Studio (Forward Mode)
Scans Ollama manifests and creates symlinks inside LM Studio:
```bash
# Interactive selection
ollama-symlinks

# Automated (links all discovered models without prompts)
ollama-symlinks --interactive=false

# Dry run (preview without modifying files)
ollama-symlinks --dry-run
```

### 3. Link LM Studio → Ollama (Reverse Mode)
Scans LM Studio GGUFs and registers them with Ollama:
```bash
# Interactive selection with default 'lms-' prefix
ollama-symlinks --reverse

# Automated with custom model prefix
ollama-symlinks --reverse --name-prefix="myorg" --interactive=false
```

### 4. Cleanup Broken Symlinks
Scans both LM Studio and Ollama blobs for orphaned symlinks (e.g. after running `ollama rm`):
```bash
# Preview broken links
ollama-symlinks cleanup --dry-run

# Interactively remove broken links
ollama-symlinks cleanup
```

### 5. Selectively Delete Symlinks
Safely remove symlinks without touching original model data:
```bash
# Remove models linked into LM Studio
ollama-symlinks delete --from lmstudio

# Remove models registered in Ollama
ollama-symlinks delete --from ollama
```

---

## ⚙️ CLI Reference

### Global Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--interactive`, `-i` | `true` | Launch interactive selection menus (`false` for automation). |
| `--reverse` | `false` | Enable Reverse Mode: Link LM Studio models to Ollama. |
| `--name-prefix` | `lms` | Prefix for models imported into Ollama (e.g. `lms-llama-3`). |
| `--ollama-dir` | *Auto* | Path to Ollama models directory (default: `~/.ollama/models`). |
| `--lmstudio-dir` | *Auto* | Path to LM Studio models directory (default: `~/.cache/lm-studio/models`). |
| `--skip-provider` | `ollama` | Provider folder name in LM Studio where links are placed. |
| `--hardlinks` | `false` | Use hard links instead of symlinks (resolves Windows permissions / 0-byte issues). |
| `--skip-checks` | `false` | Skip pre-flight executable validation checks. |
| `--dry-run` | `false` | Preview operations without creating or modifying files. |
| `--verbose` | `false` | Display verbose diagnostic logs. |
| `--deep-scan` | `false` | Scan all available Windows drives for model folders. |
| `--version` | `false` | Display binary version. |

### Subcommands

* `ollama-symlinks status` — Shows storage savings, link counts, and health.
* `ollama-symlinks cleanup` — Scans LM Studio and Ollama for broken links and removes them.
* `ollama-symlinks delete --from <lmstudio|ollama>` — Interactively selects and removes symlinks.

---

## 🗑️ Uninstallation

To remove the binary from your system:

### Automatic (macOS & Linux)
```bash
curl -fsSL https://raw.githubusercontent.com/qaribhaider/ollama-to-lmstudio-symlinks/main/uninstall.sh | sudo bash
```

### Manual
```bash
sudo rm -f /usr/local/bin/ollama-symlinks
```

*(Note: Uninstalling the binary leaves your existing symlinks and original model files intact. Use `ollama-symlinks delete` prior to removal if you wish to clean up created links first).*

---

## 📚 Documentation & Guides

- 🪟 **[Troubleshooting & Windows Guide](TROUBLESHOOTING.md)**: Windows Developer Mode, Administrator rights, hard link resolution for 0-byte models, and FAQ.
- 🛠️ **[Contributing Guide](CONTRIBUTING.md)**: Building from source, project architecture, and developer workflows.

---

## 📄 License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
