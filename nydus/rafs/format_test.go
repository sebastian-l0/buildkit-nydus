package rafs

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuperBlockWriteRead(t *testing.T) {
	sb := &SuperBlock{
		Magic:     RafsV6Magic,
		Version:   6,
		BlockSize: 256 * 1024,
	}

	var buf bytes.Buffer
	err := sb.WriteTo(&buf)
	require.NoError(t, err)

	var sb2 SuperBlock
	err = sb2.ReadFrom(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	assert.Equal(t, sb.Magic, sb2.Magic)
	assert.Equal(t, sb.Version, sb2.Version)
	assert.Equal(t, sb.BlockSize, sb2.BlockSize)
}

func TestSuperBlockValidate(t *testing.T) {
	tests := []struct {
		name    string
		sb      SuperBlock
		wantErr bool
	}{
		{
			name: "valid",
			sb: SuperBlock{
				Magic:   RafsV6Magic,
				Version: 6,
			},
			wantErr: false,
		},
		{
			name: "invalid magic",
			sb: SuperBlock{
				Magic:   0x12345678,
				Version: 6,
			},
			wantErr: true,
		},
		{
			name: "unsupported version",
			sb: SuperBlock{
				Magic:   RafsV6Magic,
				Version: 5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sb.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBootstrapBuilder(t *testing.T) {
	builder := NewBootstrapBuilder("6", 256*1024)
	require.NotNil(t, builder)

	// Add root node
	root, err := builder.AddNode("", 0o040755, 0, time.Now(), 0, 0, "")
	require.NoError(t, err)
	assert.NotNil(t, root)

	// Add a file node
	file, err := builder.AddNode("test.txt", 0o100644, 1024, time.Now(), 1000, 1000, "")
	require.NoError(t, err)
	assert.NotNil(t, file)
	assert.True(t, file.IsReg())
	assert.False(t, file.IsDir())

	// Add chunks
	chunk := ChunkInfo{
		Index:            0,
		Compressed:       false,
		Offset:           0,
		Size:             1024,
		UncompressedSize: 1024,
	}
	builder.AddChunk(file, chunk)
	assert.Len(t, file.Chunks, 1)

	// Add blob
	builder.AddBlob(BlobEntry{
		ChunkCount:       1,
		Offset:           0,
		Size:             1024,
		UncompressedSize: 1024,
		BlobID:           "test-blob",
	})

	// Build bootstrap
	var buf bytes.Buffer
	err = builder.Build(context.Background(), &buf)
	require.NoError(t, err)
	assert.Greater(t, buf.Len(), int(SuperblockOffset))

	// Verify we can read the superblock
	sbData := buf.Bytes()[int(SuperblockOffset):]
	var sb SuperBlock
	err = sb.ReadFrom(bytes.NewReader(sbData))
	require.NoError(t, err)
	assert.Equal(t, uint32(RafsV6Magic), sb.Magic)
	assert.Equal(t, uint32(6), sb.Version)
}

func TestChunker(t *testing.T) {
	chunker := NewChunker(4*1024, 256*1024, 1024*1024)
	require.NotNil(t, chunker)

	// Test chunking small data
	smallData := make([]byte, 1024)
	chunks, err := chunker.Chunk(context.Background(), smallData)
	require.NoError(t, err)
	assert.Len(t, chunks, 1)
	assert.Equal(t, uint32(1024), chunks[0].Size)

	// Test chunking larger data
	largeData := make([]byte, 5*1024*1024) // 5MB
	chunks, err = chunker.Chunk(context.Background(), largeData)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(chunks), 5)
	// With minSize=4KB, 5MB can create up to ~1280 chunks, so we just check it's reasonable
	assert.LessOrEqual(t, len(chunks), 1500)
}

func TestCompressor(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		wantErr   bool
	}{
		{"lz4_block", "lz4_block", false},
		{"gzip", "gzip", false},
		{"zstd", "zstd", false},
		{"none", "none", false},
		{"invalid", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := NewCompressor(tt.algorithm)
			assert.Equal(t, tt.algorithm, comp.Algorithm())

			data := []byte("test data for compression")
			compressed, err := comp.Compress(data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, compressed)
			}
		})
	}
}

func TestInodeTypes(t *testing.T) {
	tests := []struct {
		name     string
		mode     uint32
		wantDir  bool
		wantReg  bool
		wantLink bool
	}{
		{"directory", 0o040755, true, false, false},
		{"file", 0o100644, false, true, false},
		{"symlink", 0o120777, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inode := &Inode{Mode: tt.mode}
			assert.Equal(t, tt.wantDir, inode.IsDir())
			assert.Equal(t, tt.wantReg, inode.IsReg())
			assert.Equal(t, tt.wantLink, inode.IsSymlink())
		})
	}
}
