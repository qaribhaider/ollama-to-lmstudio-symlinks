# Ollama-LMStudio Symlinks

A Go utility to create symbolic links from Ollama models to LM Studio, allowing you to use your Ollama models in LM Studio without duplicating storage space.

## 🎯 Purpose

If you have models downloaded in Ollama and want to use them in LM Studio, you typically need to download them again, wasting disk space. This tool creates symbolic links so both applications can share the same model files.

## ✨ Features

- **🔍 Dynamic Discovery**: Automatically scans and discovers all Ollama models
- **⚙️ Configurable Paths**: Customize Ollama and LM Studio directories via command-line flags
- **🛡️ Safe Operations**: Never overwrites existing symlinks
- **📊 Clear Status**: Shows what was created vs. skipped
- **🧪 Dry Run**: Preview changes without making them
- **📝 Verbose Mode**: Detailed logging for troubleshooting
- **🎯 Multi-component Support**: Handles complex models (e.g., LLaVA with projector files)

## 🚀 Quick Start

### Run with Defaults

```bash
./ollama-symlinks
```

This will:

- Scan `~/.ollama/models` for Ollama models
- Create symlinks in `~/.cache/lm-studio/models/ollama/`

⚠️ **Important**: Do not delete the added models through LM Studio UI, else they do not show up (if symlinked again). As of now, rather go the lm studio models directory and just delete the folder of the model you want.

## 📖 Usage

### Basic Usage

```bash
# Run with default directories
./ollama-symlinks

# Dry run to see what would happen (recommended first run)
./ollama-symlinks -dry-run

# Enable verbose output
./ollama-symlinks -verbose
```

### Custom Directories

```bash
# Specify custom Ollama directory
./ollama-symlinks -ollama-dir="/path/to/ollama/models"

# Specify custom LM Studio directory
./ollama-symlinks -lmstudio-dir="/path/to/lmstudio/models"

# Both custom directories
./ollama-symlinks -ollama-dir="/custom/ollama" -lmstudio-dir="/custom/lmstudio"
```

### All Options

```bash
./ollama-symlinks -help

Flags:
  -ollama-dir string     Path to Ollama models directory (default "~/.ollama/models")
  -lmstudio-dir string   Path to LM Studio models directory (default "~/.cache/lm-studio/models")
  -dry-run              Show what would be done without making changes
  -verbose              Enable verbose output
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
go build -ldflags="-X 'main.Version=$(cat VERSION)'" -o ollama-symlinks ollama-symlinks.go

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

1. **Scans** the Ollama manifests directory (`~/.ollama/models/manifests/`)
2. **Parses** JSON manifest files to identify model components
3. **Maps** model names from Ollama's format to LM Studio-friendly names
4. **Creates** an "ollama" provider directory in LM Studio
5. **Generates** symbolic links with proper `.gguf` extensions
6. **Handles** multi-component models (like LLaVA with projector files)

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

- Go 1.16+ (uses only standard library)

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
- **Deletion in Lm Studio**: Do not delete the added models through LM Studio UI, else they do not show up again (if symlinked). As of now, rather go the lm studio models directory and just delete the folder of the model you want.

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
