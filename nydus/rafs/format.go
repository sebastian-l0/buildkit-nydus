// Package rafs provides RAFS (Registry Accelerated File System) format support
package rafs

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/opencontainers/go-digest"
)

// RAFS v6 superblock constants
const (
	// Magic number for RAFS v6
	RafsV6Magic = 0x52414653 // "RAFS"

	// Superblock offset
	SuperblockOffset = 1024

	// Chunk size limits
	MinChunkSize = 4 * 1024       // 4KB
	MaxChunkSize = 1024 * 1024    // 1MB
	DefaultChunkSize = 256 * 1024 // 256KB
)

// SuperBlock represents the RAFS v6 superblock
type SuperBlock struct {
	Magic           uint32
	Version         uint32
	Flags           uint32
	BlockSize       uint32
	MetaOffset      uint64
	MetaSize        uint64
	PrefetchOffset  uint64
	PrefetchSize    uint64
	BlobTableOffset uint64
	BlobTableSize   uint64
	InodeTableOffset uint64
	InodeTableSize  uint64
	ChunkTableOffset uint64
	ChunkTableSize  uint64
	XattrTableOffset uint64
	XattrTableSize  uint64
	Reserved        [32]byte
	CheckOffset     uint64
	CheckSize       uint64
}

// WriteTo writes the superblock to a writer
func (sb *SuperBlock) WriteTo(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, sb)
}

// ReadFrom reads the superblock from a reader
func (sb *SuperBlock) ReadFrom(r io.Reader) error {
	return binary.Read(r, binary.LittleEndian, sb)
}

// Validate checks if the superblock is valid
func (sb *SuperBlock) Validate() error {
	if sb.Magic != RafsV6Magic {
		return fmt.Errorf("invalid magic: expected 0x%x, got 0x%x", RafsV6Magic, sb.Magic)
	}
	if sb.Version != 6 {
		return fmt.Errorf("unsupported version: %d", sb.Version)
	}
	return nil
}

// ChunkInfo represents a chunk in the blob
type ChunkInfo struct {
	Index      uint32
	Compressed bool
	Offset     uint64
	Size       uint32
	UncompressedSize uint32
	Digest     digest.Digest
}

// BlobEntry represents a blob in the blob table
type BlobEntry struct {
	ChunkCount  uint32
	ReadAhead   uint32
	Offset      uint64
	Size        uint64
	UncompressedSize uint64
	Digest      digest.Digest
	BlobID      string
	Meta        map[string]string
}

// Inode represents a file/directory inode
type Inode struct {
	Mode        uint32
	Uid         uint32
	Gid         uint32
	Mtime       uint64
	Size        uint64
	Nlink       uint32
	XattrCount  uint32
	XattrOffset uint32
	Name        string
	Digest      digest.Digest
	Chunks      []ChunkInfo
}

// IsDir returns true if the inode is a directory
func (i *Inode) IsDir() bool {
	return i.Mode&0o040000 != 0
}

// IsReg returns true if the inode is a regular file
func (i *Inode) IsReg() bool {
	return i.Mode&0o100000 != 0
}

// IsSymlink returns true if the inode is a symlink
func (i *Inode) IsSymlink() bool {
	return i.Mode&0o120000 != 0
}

// CalculateCrc32 calculates CRC32 checksum for data
func CalculateCrc32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// FormatVersion returns the supported RAFS version
func FormatVersion() int {
	return 6
}
