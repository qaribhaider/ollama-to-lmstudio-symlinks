package gguf

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// helper to build a minimal synthetic GGUF buffer
func createSyntheticGGUF(version uint32, kvs map[string]interface{}) []byte {
	buf := new(bytes.Buffer)

	// Magic: GGUF
	buf.Write([]byte{'G', 'G', 'U', 'F'})

	// Version
	binary.Write(buf, binary.LittleEndian, version)

	// Tensor count
	binary.Write(buf, binary.LittleEndian, uint64(10))

	// Metadata KV count
	binary.Write(buf, binary.LittleEndian, uint64(len(kvs)))

	for k, v := range kvs {
		// Write key
		binary.Write(buf, binary.LittleEndian, uint64(len(k)))
		buf.WriteString(k)

		switch val := v.(type) {
		case string:
			binary.Write(buf, binary.LittleEndian, TypeString)
			binary.Write(buf, binary.LittleEndian, uint64(len(val)))
			buf.WriteString(val)
		case uint32:
			binary.Write(buf, binary.LittleEndian, TypeUint32)
			binary.Write(buf, binary.LittleEndian, val)
		case bool:
			binary.Write(buf, binary.LittleEndian, TypeBool)
			var b byte
			if val {
				b = 1
			}
			buf.WriteByte(b)
		case []string:
			binary.Write(buf, binary.LittleEndian, TypeArray)
			binary.Write(buf, binary.LittleEndian, TypeString)
			binary.Write(buf, binary.LittleEndian, uint64(len(val)))
			for _, item := range val {
				binary.Write(buf, binary.LittleEndian, uint64(len(item)))
				buf.WriteString(item)
			}
		}
	}

	return buf.Bytes()
}

func TestInspectGGUFReader_BaseModel(t *testing.T) {
	data := createSyntheticGGUF(3, map[string]interface{}{
		"general.architecture": "llama",
		"llama.context_length": uint32(4096),
	})

	info, err := InspectGGUFReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Architecture != "llama" {
		t.Errorf("expected architecture 'llama', got '%s'", info.Architecture)
	}
	if info.IsProjector {
		t.Errorf("expected IsProjector to be false, got true")
	}
	if info.Version != 3 {
		t.Errorf("expected version 3, got %d", info.Version)
	}
}

func TestInspectGGUFReader_ClipProjector(t *testing.T) {
	data := createSyntheticGGUF(3, map[string]interface{}{
		"general.architecture": "clip",
	})

	info, err := InspectGGUFReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Architecture != "clip" {
		t.Errorf("expected architecture 'clip', got '%s'", info.Architecture)
	}
	if !info.IsProjector {
		t.Errorf("expected IsProjector to be true, got false")
	}
}

func TestInspectGGUFReader_SiglipProjector(t *testing.T) {
	data := createSyntheticGGUF(3, map[string]interface{}{
		"general.architecture": "siglip",
	})

	info, err := InspectGGUFReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Architecture != "siglip" {
		t.Errorf("expected architecture 'siglip', got '%s'", info.Architecture)
	}
	if !info.IsProjector {
		t.Errorf("expected IsProjector to be true, got false")
	}
}

func TestInspectGGUFReader_ClipKeyPrefix(t *testing.T) {
	data := createSyntheticGGUF(2, map[string]interface{}{
		"clip.projector_type": "mlp",
		"general.name":        "my-clip-model",
	})

	info, err := InspectGGUFReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !info.IsProjector {
		t.Errorf("expected IsProjector to be true due to clip.projector_type, got false")
	}
}

func TestInspectGGUFReader_InvalidMagic(t *testing.T) {
	data := []byte{'N', 'O', 'P', 'E', 0, 0, 0, 3}
	_, err := InspectGGUFReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for invalid magic, got nil")
	}
}

func TestInspectGGUFReader_UnsupportedVersion(t *testing.T) {
	data := createSyntheticGGUF(1, map[string]interface{}{
		"general.architecture": "llama",
	})
	_, err := InspectGGUFReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for unsupported version 1, got nil")
	}
}

func TestInspectGGUFReader_Truncated(t *testing.T) {
	data := []byte{'G', 'G'}
	_, err := InspectGGUFReader(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error for truncated reader, got nil")
	}
}

func TestInspectGGUF_File(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.gguf")

	data := createSyntheticGGUF(3, map[string]interface{}{
		"general.architecture": "qwen2",
	})
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	info, err := InspectGGUF(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Architecture != "qwen2" {
		t.Errorf("expected architecture 'qwen2', got '%s'", info.Architecture)
	}
	if info.IsProjector {
		t.Errorf("expected IsProjector to be false, got true")
	}
}

func TestInspectGGUF_NonExistentFile(t *testing.T) {
	_, err := InspectGGUF("/non/existent/file.gguf")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}
