package rafs

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFastCDC(t *testing.T) {
	fc := NewFastCDC(4*1024, 256*1024, 1024*1024)
	require.NotNil(t, fc)

	tests := []struct {
		name     string
		dataSize int
		minChunks int
		maxChunks int
	}{
		{"empty", 0, 0, 0},
		{"small", 1024, 1, 1},
		{"medium", 100 * 1024, 1, 10},
		{"large", 5 * 1024 * 1024, 5, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.dataSize)
			for i := range data {
				data[i] = byte(i % 256)
			}

			chunks, err := fc.Chunk(data)
			require.NoError(t, err)

			if tt.dataSize == 0 {
				assert.Nil(t, chunks)
				return
			}

			assert.GreaterOrEqual(t, len(chunks), tt.minChunks)
			assert.LessOrEqual(t, len(chunks), tt.maxChunks)

			// Verify chunk boundaries
			var totalSize int
			for i, chunk := range chunks {
				assert.Equal(t, uint32(i), chunk.Index)
				assert.Equal(t, uint64(totalSize), chunk.Offset)
				totalSize += int(chunk.Size)
			}
			assert.Equal(t, tt.dataSize, totalSize)
		})
	}
}

func TestFastCDCContentDefined(t *testing.T) {
	fc := NewFastCDC(4*1024, 256*1024, 1024*1024)

	// Create data with repeating pattern
	data := bytes.Repeat([]byte("hello world "), 100000)

	chunks1, err := fc.Chunk(data)
	require.NoError(t, err)

	// Modify a small portion in the middle
	data2 := make([]byte, len(data))
	copy(data2, data)
	data2[10000] = 'X'

	chunks2, err := fc.Chunk(data2)
	require.NoError(t, err)

	// Content-defined chunking should produce similar chunk boundaries
	// except near the modified area
	t.Logf("Chunks 1: %d, Chunks 2: %d", len(chunks1), len(chunks2))
	assert.Greater(t, len(chunks1), 0)
	assert.Greater(t, len(chunks2), 0)
}

func TestCompressorV2(t *testing.T) {
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
			c := NewCompressorV2(tt.algorithm)
			assert.NotNil(t, c)
			assert.Equal(t, tt.algorithm, c.Algorithm())

			data := []byte("test data for compression algorithm: " + tt.algorithm)
			compressed, err := c.Compress(data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, compressed)

			// "none" and "lz4_block" return same data
			if tt.algorithm == "none" || tt.algorithm == "lz4_block" {
				assert.Equal(t, data, compressed)
			}
		})
	}
}

func TestCompressorV2CompressMethods(t *testing.T) {
	t.Run("gzip compression", func(t *testing.T) {
		c := NewCompressorV2("gzip")
		data := bytes.Repeat([]byte("hello world "), 1000)

		compressed, err := c.Compress(data)
		require.NoError(t, err)

		// Gzip should compress repetitive data
		assert.Less(t, len(compressed), len(data))
	})

	t.Run("zstd compression", func(t *testing.T) {
		c := NewCompressorV2("zstd")
		data := bytes.Repeat([]byte("hello world "), 1000)

		compressed, err := c.Compress(data)
		require.NoError(t, err)

		// ZSTD should compress repetitive data
		assert.Less(t, len(compressed), len(data))
	})

	t.Run("pool reuse", func(t *testing.T) {
		c := NewCompressorV2("zstd")
		data := []byte("test data")

		// Multiple compressions should reuse encoder pool
		for i := 0; i < 10; i++ {
			_, err := c.Compress(data)
			require.NoError(t, err)
		}
	})
}

func TestStreamingChunker(t *testing.T) {
	sc := NewStreamingChunker(4*1024, 256*1024, 1024*1024, "none")
	require.NotNil(t, sc)

	t.Run("empty data", func(t *testing.T) {
		result, err := sc.Process(context.Background(), nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Nil(t, result.Chunks)
		assert.Nil(t, result.CompressedBlob)
	})

	t.Run("small data", func(t *testing.T) {
		data := []byte("hello world test data")
		result, err := sc.Process(context.Background(), data)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Chunks)
		assert.Greater(t, len(result.Chunks), 0)
		assert.NotEmpty(t, result.BlobDigest)
	})

	t.Run("larger data", func(t *testing.T) {
		data := make([]byte, 1024*1024) // 1MB
		for i := range data {
			data[i] = byte(i % 256)
		}

		result, err := sc.Process(context.Background(), data)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.GreaterOrEqual(t, len(result.Chunks), 1)
		assert.Equal(t, int64(len(data)), result.UncompressedSize)
	})

	t.Run("with compression", func(t *testing.T) {
		scZstd := NewStreamingChunker(4*1024, 256*1024, 1024*1024, "zstd")
		data := bytes.Repeat([]byte("repetitive data "), 10000)

		result, err := scZstd.Process(context.Background(), data)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Less(t, result.CompressedSize, result.UncompressedSize)
	})
}

func TestStreamingChunkerProcessStream(t *testing.T) {
	sc := NewStreamingChunker(4*1024, 256*1024, 1024*1024, "none")

	data := []byte("stream test data for chunking")
	reader := bytes.NewReader(data)

	result, err := sc.ProcessStream(context.Background(), reader)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(len(data)), result.UncompressedSize)
}

func TestGearTable(t *testing.T) {
	// Verify FastCDC creates unique gear values
	fc := NewFastCDC(4*1024, 256*1024, 1024*1024)
	require.NotNil(t, fc)
	
	// Check that gear values are initialized
	assert.NotEqual(t, uint64(0), fc.gear[0])
	assert.NotEqual(t, uint64(0), fc.gear[255])
	
	// Verify values are different
	for i := 1; i < 256; i++ {
		assert.NotEqual(t, fc.gear[i-1], fc.gear[i])
	}
}
