package rafs

import (
	"context"
	"fmt"
	"io"
)

// Chunker handles file chunking for Nydus
type Chunker struct {
	minSize    int
	avgSize    int
	maxSize    int
	hashWindow int
}

// NewChunker creates a new chunker with the given parameters
func NewChunker(minSize, avgSize, maxSize int) *Chunker {
	return &Chunker{
		minSize:    minSize,
		avgSize:    avgSize,
		maxSize:    maxSize,
		hashWindow: 4096,
	}
}

// Chunk splits data into chunks using FastCDC algorithm
func (c *Chunker) Chunk(ctx context.Context, data []byte) ([]ChunkInfo, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var chunks []ChunkInfo
	var offset int
	index := uint32(0)

	for offset < len(data) {
		remaining := len(data) - offset
		
		// Determine chunk size
		chunkSize := remaining
		if chunkSize > c.maxSize {
			chunkSize = c.maxSize
		}
		if chunkSize > c.minSize && remaining > c.maxSize {
			// Use FastCDC to find a cut point
			cutPoint := c.findCutPoint(data[offset:], c.minSize, c.avgSize, c.maxSize)
			if cutPoint > 0 {
				chunkSize = cutPoint
			}
		}

		chunk := ChunkInfo{
			Index:            index,
			Offset:           uint64(offset),
			Size:             uint32(chunkSize),
			UncompressedSize: uint32(chunkSize),
		}
		chunks = append(chunks, chunk)

		offset += chunkSize
		index++
	}

	return chunks, nil
}

// ChunkStream chunks data from a reader
func (c *Chunker) ChunkStream(ctx context.Context, r io.Reader) ([]ChunkInfo, []byte, error) {
	// Read all data first (for simplicity, can be optimized for streaming)
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read data: %w", err)
	}

	chunks, err := c.Chunk(ctx, data)
	if err != nil {
		return nil, nil, err
	}

	return chunks, data, nil
}

// findCutPoint finds a suitable cut point using FastCDC algorithm
func (c *Chunker) findCutPoint(data []byte, minSize, avgSize, maxSize int) int {
	if len(data) < minSize {
		return len(data)
	}

	// Simple rolling hash for now
	// In production, use proper FastCDC implementation
	target := avgSize
	if target > len(data) {
		target = len(data)
	}

	// Geometric average mask
	mask := uint64((1 << 20) - 1) // Approximate for avgSize around 256KB

	var hash uint64 = 0
	for i := minSize; i < len(data) && i < maxSize; i++ {
		hash = (hash << 1) + uint64(data[i])
		if (hash & mask) == 0 {
			return i
		}
	}

	return maxSize
}

// Compressor handles blob compression
type Compressor struct {
	algorithm string
}

// NewCompressor creates a new compressor
func NewCompressor(algorithm string) *Compressor {
	return &Compressor{algorithm: algorithm}
}

// Compress compresses data using the configured algorithm
func (c *Compressor) Compress(data []byte) ([]byte, error) {
	switch c.algorithm {
	case "lz4_block":
		// For now, return uncompressed (implement lz4 later)
		return data, nil
	case "gzip":
		// Implement gzip compression
		return c.compressGzip(data)
	case "zstd":
		// Implement zstd compression
		return c.compressZstd(data)
	case "none":
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported compressor: %s", c.algorithm)
	}
}

func (c *Compressor) compressGzip(data []byte) ([]byte, error) {
	// Placeholder - implement actual gzip compression
	return data, nil
}

func (c *Compressor) compressZstd(data []byte) ([]byte, error) {
	// Placeholder - implement actual zstd compression
	return data, nil
}

// Algorithm returns the compression algorithm
func (c *Compressor) Algorithm() string {
	return c.algorithm
}
