package gguf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	// GGUFMagic represents the 4-byte magic number "GGUF" in little-endian ASCII.
	GGUFMagic = [4]byte{'G', 'G', 'U', 'F'}

	ErrInvalidMagic   = errors.New("not a valid GGUF file: invalid magic bytes")
	ErrUnsupportedVer = errors.New("unsupported GGUF version")
)

// GGUF metadata value types
const (
	TypeUint8   uint32 = 0
	TypeInt8    uint32 = 1
	TypeUint16  uint32 = 2
	TypeInt16   uint32 = 3
	TypeUint32  uint32 = 4
	TypeInt32   uint32 = 5
	TypeFloat32 uint32 = 6
	TypeBool    uint32 = 7
	TypeString  uint32 = 8
	TypeArray   uint32 = 9
	TypeUint64  uint32 = 10
	TypeInt64   uint32 = 11
	TypeFloat64 uint32 = 12
)

// GGUFInfo stores metadata extracted from a GGUF header.
type GGUFInfo struct {
	Version      uint32
	Architecture string
	IsProjector  bool
}

// Known projector architectures commonly found in multimodal/vision GGUF files.
var projectorArchitectures = map[string]bool{
	"clip":      true,
	"siglip":    true,
	"mplug-owl": true,
	"vision":    true,
}

// IsProjectorArchitecture returns true if the architecture is a known multimodal vision projector.
func IsProjectorArchitecture(arch string) bool {
	return projectorArchitectures[strings.ToLower(strings.TrimSpace(arch))]
}

// InspectGGUF opens a file and parses its GGUF header to extract architecture metadata.
// It only reads the header and metadata key-value store, stopping early once the architecture
// is identified or the metadata is exhausted.
func InspectGGUF(path string) (*GGUFInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return InspectGGUFReader(f)
}

// InspectGGUFReader parses GGUF header metadata from any io.Reader.
func InspectGGUFReader(r io.Reader) (*GGUFInfo, error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMagic, err)
	}
	if magic != GGUFMagic {
		return nil, ErrInvalidMagic
	}

	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}
	if version < 2 || version > 3 {
		return nil, fmt.Errorf("%w: version %d", ErrUnsupportedVer, version)
	}

	var tensorCount uint64
	if err := binary.Read(r, binary.LittleEndian, &tensorCount); err != nil {
		return nil, fmt.Errorf("reading tensor count: %w", err)
	}

	var metadataKVCount uint64
	if err := binary.Read(r, binary.LittleEndian, &metadataKVCount); err != nil {
		return nil, fmt.Errorf("reading metadata count: %w", err)
	}

	info := &GGUFInfo{
		Version: version,
	}

	// Iterate through metadata KV pairs.
	// Safety limit: avoid scanning more than 10,000 keys to prevent hangs on malformed inputs.
	maxKeys := metadataKVCount
	if maxKeys > 10000 {
		maxKeys = 10000
	}

	for i := uint64(0); i < maxKeys; i++ {
		key, err := readString(r)
		if err != nil {
			// If we reached EOF or error but already found architecture, return what we have
			if info.Architecture != "" {
				break
			}
			return nil, fmt.Errorf("reading key %d: %w", i, err)
		}

		var valType uint32
		if err := binary.Read(r, binary.LittleEndian, &valType); err != nil {
			if info.Architecture != "" {
				break
			}
			return nil, fmt.Errorf("reading value type for key '%s': %w", key, err)
		}

		if key == "general.architecture" && valType == TypeString {
			arch, err := readString(r)
			if err != nil {
				return nil, fmt.Errorf("reading general.architecture value: %w", err)
			}
			info.Architecture = arch
			if IsProjectorArchitecture(arch) {
				info.IsProjector = true
			}
			// We have found the architecture! We can return immediately.
			return info, nil
		}

		// Also check for clip projector indicator keys in case architecture was omitted or generic
		if strings.HasPrefix(key, "clip.") {
			info.IsProjector = true
		}

		// Skip value based on type
		if err := skipValue(r, valType); err != nil {
			if info.Architecture != "" {
				break
			}
			return nil, fmt.Errorf("skipping value for key '%s': %w", key, err)
		}
	}

	return info, nil
}

func readString(r io.Reader) (string, error) {
	var length uint64
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	// Sanity limit: keys and strings in GGUF metadata should not exceed 1MB
	if length > 1024*1024 {
		return "", fmt.Errorf("string length exceeds 1MB limit: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func skipValue(r io.Reader, valType uint32) error {
	switch valType {
	case TypeUint8, TypeInt8, TypeBool:
		var dummy [1]byte
		_, err := io.ReadFull(r, dummy[:])
		return err
	case TypeUint16, TypeInt16:
		var dummy [2]byte
		_, err := io.ReadFull(r, dummy[:])
		return err
	case TypeUint32, TypeInt32, TypeFloat32:
		var dummy [4]byte
		_, err := io.ReadFull(r, dummy[:])
		return err
	case TypeUint64, TypeInt64, TypeFloat64:
		var dummy [8]byte
		_, err := io.ReadFull(r, dummy[:])
		return err
	case TypeString:
		var length uint64
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return err
		}
		if length > 10*1024*1024 {
			return fmt.Errorf("string length too large: %d", length)
		}
		return discardBytes(r, length)
	case TypeArray:
		var elemType uint32
		if err := binary.Read(r, binary.LittleEndian, &elemType); err != nil {
			return err
		}
		var elemCount uint64
		if err := binary.Read(r, binary.LittleEndian, &elemCount); err != nil {
			return err
		}
		if elemCount > 1000000 {
			return fmt.Errorf("array element count too large: %d", elemCount)
		}
		for i := uint64(0); i < elemCount; i++ {
			if err := skipValue(r, elemType); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown GGUF value type: %d", valType)
	}
}

func discardBytes(r io.Reader, count uint64) error {
	if seeker, ok := r.(io.Seeker); ok {
		_, err := seeker.Seek(int64(count), io.SeekCurrent)
		return err
	}
	// Fallback to discarding via copy into a discard buffer
	lr := io.LimitReader(r, int64(count))
	copied, err := io.Copy(io.Discard, lr)
	if err != nil {
		return err
	}
	if uint64(copied) != count {
		return io.ErrUnexpectedEOF
	}
	return nil
}
