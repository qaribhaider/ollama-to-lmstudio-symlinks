# Troubleshooting & Platform Guide

This guide covers common platform-specific configurations, permissions, and troubleshooting steps for `ollama-symlinks`.

---

## 🪟 Windows Specifics

### 1. "Permission denied" Creating Symbolic Links
By default, Windows restricts symbolic link creation to Administrator accounts or accounts with **Developer Mode** enabled.

**Solutions:**
* **Option A (Recommended)**: Enable **Developer Mode** in Windows Settings (`Settings > System > For developers > Developer Mode`). This allows creating symlinks without admin privileges.
* **Option B**: Run your terminal (PowerShell or Command Prompt) as **Administrator**.
* **Option C**: Use hard links instead via the `--hardlinks` flag:
  ```bash
  ollama-symlinks --hardlinks
  ```

---

### 2. "Failed to load model" or Models Showing as "0 Bytes" in LM Studio
On some Windows configurations, LM Studio cannot read standard Windows directory or file symlinks, displaying them as 0 bytes.

**Resolution:**
1. Remove the existing 0-byte links:
   ```bash
   ollama-symlinks delete --from lmstudio
   ```
2. Re-link using the `--hardlinks` flag:
   ```bash
   ollama-symlinks --hardlinks
   ```

> [!IMPORTANT]
> **How Hard Links Affect Disk Space**:
> Hard links point directly to the file data on the NTFS volume. If you run `ollama rm <model>`, disk space **will not be freed** until the linked file in LM Studio is also deleted via `ollama-symlinks delete --from lmstudio`.

---

## 🔍 Common Issues

### 1. "Blocking validation error: 'ollama' executable not found" or "LM Studio not found"
Before making changes, the utility verifies that Ollama and LM Studio are accessible on your system.

If you are using portable installations, custom paths, or running in headless/container environments, bypass pre-flight checks with `--skip-checks`:

```bash
# Bypass checks in forward mode
ollama-symlinks --skip-checks

# Bypass checks with custom directories
ollama-symlinks --ollama-dir="D:\Ollama\models" --lmstudio-dir="D:\LMStudio\models" --skip-checks
```

---

### 2. Models Not Appearing in LM Studio
1. **Reload LM Studio**: Restart LM Studio or press `Cmd+R` / `Ctrl+R` to force a library index refresh.
2. **Verify Provider Directory**: Check that links exist inside your LM Studio models directory:
   * **macOS/Linux**: `ls -la ~/.cache/lm-studio/models/ollama/`
   * **Windows**: `dir "%USERPROFILE%\.cache\lm-studio\models\ollama"`
3. **Inspect Status**: Run the status command to check link health:
   ```bash
   ollama-symlinks status --verbose
   ```

---

### 3. "No models found" in Ollama
Verify your Ollama manifests directory exists and contains models:
* **macOS/Linux**: `ls -la ~/.ollama/models/manifests/`
* **Windows**: `dir "%USERPROFILE%\.ollama\models\manifests"`

If your models are stored in a non-default location (e.g. via `OLLAMA_MODELS`), pass `--ollama-dir`:
```bash
ollama-symlinks --ollama-dir="/custom/path/to/models"
```

---

### 4. Broken Symlinks After Running `ollama rm`
When an Ollama model is removed with `ollama rm`, its underlying blob is deleted, leaving the symlink in LM Studio pointing to a missing target.

Run the cleanup command to automatically find and purge broken links across both LM Studio and Ollama:
```bash
# Preview broken links
ollama-symlinks cleanup --dry-run

# Remove broken links
ollama-symlinks cleanup
```
