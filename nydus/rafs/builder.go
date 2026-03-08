package rafs

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/opencontainers/go-digest"
)

// BootstrapBuilder builds the RAFS bootstrap (metadata)
type BootstrapBuilder struct {
	fsVersion    string
	chunkSize    int
	nodes        map[uint64]*Inode
	root         *Inode
	nextInodeNum uint64
	blobTable    []BlobEntry
}

// NewBootstrapBuilder creates a new bootstrap builder
func NewBootstrapBuilder(fsVersion string, chunkSize int) *BootstrapBuilder {
	return &BootstrapBuilder{
		fsVersion:    fsVersion,
		chunkSize:    chunkSize,
		nodes:        make(map[uint64]*Inode),
		nextInodeNum: 1,
	}
}

// AddNode adds a file/directory node to the bootstrap
func (bb *BootstrapBuilder) AddNode(path string, mode uint32, size uint64, modTime time.Time, uid, gid uint32, target string) (*Inode, error) {
	inode := &Inode{
		Mode:  mode,
		Uid:   uid,
		Gid:   gid,
		Mtime: uint64(modTime.Unix()),
		Size:  size,
		Nlink: 1,
		Name:  path,
	}

	if mode&0o120000 != 0 {
		// Symlink
		inode.Size = uint64(len(target))
	}

	inodeNum := bb.nextInodeNum
	bb.nextInodeNum++
	bb.nodes[inodeNum] = inode

	if path == "" || path == "/" {
		bb.root = inode
	}

	return inode, nil
}

// AddChunk adds a chunk to an inode
func (bb *BootstrapBuilder) AddChunk(inode *Inode, chunk ChunkInfo) {
	inode.Chunks = append(inode.Chunks, chunk)
}

// AddBlob adds a blob entry to the blob table
func (bb *BootstrapBuilder) AddBlob(entry BlobEntry) {
	bb.blobTable = append(bb.blobTable, entry)
}

// Build builds the bootstrap and writes it to the writer
func (bb *BootstrapBuilder) Build(ctx context.Context, w io.Writer) error {
	if bb.root == nil {
		return fmt.Errorf("no root inode")
	}

	// Sort inodes by path for consistent output
	var sortedInodes []*Inode
	for _, inode := range bb.nodes {
		sortedInodes = append(sortedInodes, inode)
	}
	sort.Slice(sortedInodes, func(i, j int) bool {
		return sortedInodes[i].Name < sortedInodes[j].Name
	})

	// Calculate offsets
	var (
		inodeTableSize   = bb.calculateInodeTableSize(sortedInodes)
		chunkTableSize   = bb.calculateChunkTableSize(sortedInodes)
		blobTableSize    = bb.calculateBlobTableSize()
		xattrTableSize   = bb.calculateXattrTableSize(sortedInodes)
		nameTableSize    = bb.calculateNameTableSize(sortedInodes)
	)

	metaOffset := uint64(SuperblockOffset + 256) // Leave room for superblock
	inodeTableOffset := metaOffset
	chunkTableOffset := inodeTableOffset + uint64(inodeTableSize)
	blobTableOffset := chunkTableOffset + uint64(chunkTableSize)
	xattrTableOffset := blobTableOffset + uint64(blobTableSize)
	nameTableOffset := xattrTableOffset + uint64(xattrTableSize)
	metaSize := nameTableOffset + uint64(nameTableSize) - metaOffset

	// Write superblock
	sb := &SuperBlock{
		Magic:            RafsV6Magic,
		Version:          6,
		BlockSize:        uint32(bb.chunkSize),
		MetaOffset:       metaOffset,
		MetaSize:         metaSize,
		InodeTableOffset: inodeTableOffset,
		InodeTableSize:   uint64(inodeTableSize),
		ChunkTableOffset: chunkTableOffset,
		ChunkTableSize:   uint64(chunkTableSize),
		BlobTableOffset:  blobTableOffset,
		BlobTableSize:    uint64(blobTableSize),
		XattrTableOffset: xattrTableOffset,
		XattrTableSize:   uint64(xattrTableSize),
	}

	// Pad to superblock offset
	if _, err := w.Write(make([]byte, SuperblockOffset)); err != nil {
		return fmt.Errorf("failed to write padding: %w", err)
	}

	if err := sb.WriteTo(w); err != nil {
		return fmt.Errorf("failed to write superblock: %w", err)
	}

	// Write inode table
	if err := bb.writeInodeTable(w, sortedInodes, chunkTableOffset, xattrTableOffset); err != nil {
		return fmt.Errorf("failed to write inode table: %w", err)
	}

	// Write chunk table
	if err := bb.writeChunkTable(w, sortedInodes); err != nil {
		return fmt.Errorf("failed to write chunk table: %w", err)
	}

	// Write blob table
	if err := bb.writeBlobTable(w); err != nil {
		return fmt.Errorf("failed to write blob table: %w", err)
	}

	// Write xattr table
	if err := bb.writeXattrTable(w, sortedInodes); err != nil {
		return fmt.Errorf("failed to write xattr table: %w", err)
	}

	// Write name table
	if err := bb.writeNameTable(w, sortedInodes); err != nil {
		return fmt.Errorf("failed to write name table: %w", err)
	}

	return nil
}

func (bb *BootstrapBuilder) calculateInodeTableSize(inodes []*Inode) int {
	// Each inode entry is fixed size for simplicity
	return len(inodes) * 128
}

func (bb *BootstrapBuilder) calculateChunkTableSize(inodes []*Inode) int {
	total := 0
	for _, inode := range inodes {
		total += len(inode.Chunks) * 48 // Each chunk entry is ~48 bytes
	}
	return total
}

func (bb *BootstrapBuilder) calculateBlobTableSize() int {
	return len(bb.blobTable) * 256 // Each blob entry is ~256 bytes
}

func (bb *BootstrapBuilder) calculateXattrTableSize(inodes []*Inode) int {
	// Simplified: assume no xattrs for now
	return 0
}

func (bb *BootstrapBuilder) calculateNameTableSize(inodes []*Inode) int {
	total := 0
	for _, inode := range inodes {
		total += len(inode.Name) + 1 // +1 for null terminator
	}
	return total
}

func (bb *BootstrapBuilder) writeInodeTable(w io.Writer, inodes []*Inode, chunkOffset, xattrOffset uint64) error {
	for _, inode := range inodes {
		entry := struct {
			Mode        uint32
			Uid         uint32
			Gid         uint32
			Mtime       uint64
			Size        uint64
			Nlink       uint32
			XattrCount  uint32
			XattrOffset uint64
			ChunkCount  uint32
			ChunkOffset uint64
			NameOffset  uint32
			NameLen     uint32
		}{
			Mode:        inode.Mode,
			Uid:         inode.Uid,
			Gid:         inode.Gid,
			Mtime:       inode.Mtime,
			Size:        inode.Size,
			Nlink:       inode.Nlink,
			XattrCount:  inode.XattrCount,
			XattrOffset: xattrOffset,
			ChunkCount:  uint32(len(inode.Chunks)),
			ChunkOffset: chunkOffset,
		}
		if err := binary.Write(w, binary.LittleEndian, entry); err != nil {
			return err
		}
	}
	return nil
}

func (bb *BootstrapBuilder) writeChunkTable(w io.Writer, inodes []*Inode) error {
	for _, inode := range inodes {
		for _, chunk := range inode.Chunks {
			entry := struct {
				Index      uint32
				Compressed uint32
				Offset     uint64
				Size       uint32
				UncompressedSize uint32
				BlobIndex  uint32
				Reserved   uint32
			}{
				Index:      chunk.Index,
				Compressed: 0,
				Offset:     chunk.Offset,
				Size:       chunk.Size,
				UncompressedSize: chunk.UncompressedSize,
				BlobIndex:  0,
			}
			if chunk.Compressed {
				entry.Compressed = 1
			}
			if err := binary.Write(w, binary.LittleEndian, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (bb *BootstrapBuilder) writeBlobTable(w io.Writer) error {
	for _, blob := range bb.blobTable {
		entry := struct {
			ChunkCount       uint32
			ReadAhead        uint32
			Offset           uint64
			Size             uint64
			UncompressedSize uint64
			BlobIDLen        uint32
			Reserved         uint32
		}{
			ChunkCount:       blob.ChunkCount,
			ReadAhead:        blob.ReadAhead,
			Offset:           blob.Offset,
			Size:             blob.Size,
			UncompressedSize: blob.UncompressedSize,
			BlobIDLen:        uint32(len(blob.BlobID)),
		}
		if err := binary.Write(w, binary.LittleEndian, entry); err != nil {
			return err
		}
		// Write blob ID
		if _, err := io.WriteString(w, blob.BlobID); err != nil {
			return err
		}
		// Write null terminator
		if _, err := w.Write([]byte{0}); err != nil {
			return err
		}
	}
	return nil
}

func (bb *BootstrapBuilder) writeXattrTable(w io.Writer, inodes []*Inode) error {
	// Simplified: no xattrs for now
	return nil
}

func (bb *BootstrapBuilder) writeNameTable(w io.Writer, inodes []*Inode) error {
	for _, inode := range inodes {
		if _, err := io.WriteString(w, inode.Name); err != nil {
			return err
		}
		if _, err := w.Write([]byte{0}); err != nil {
			return err
		}
	}
	return nil
}

// GetDigest calculates the digest of the bootstrap
func (bb *BootstrapBuilder) GetDigest() (digest.Digest, error) {
	var buf bytes.Buffer
	if err := bb.Build(context.Background(), &buf); err != nil {
		return "", err
	}
	return digest.FromBytes(buf.Bytes()), nil
}
