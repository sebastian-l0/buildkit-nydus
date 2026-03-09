package rafs

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/opencontainers/go-digest"
)

// FastCDC implements the Fast Content-Defined Chunking algorithm
// Based on: https://www.usenix.org/system/files/conference/atc16/atc16-paper-xia.pdf
type FastCDC struct {
	minSize    int
	avgSize    int
	maxSize    int
	maskS      uint64 // Mask for small chunks
	maskL      uint64 // Mask for large chunks
	gear       [256]uint64
}

// NewFastCDC creates a new FastCDC chunker
func NewFastCDC(minSize, avgSize, maxSize int) *FastCDC {
	fc := &FastCDC{
		minSize: minSize,
		avgSize: avgSize,
		maxSize: maxSize,
	}

	// Initialize gear table with pseudo-random values
	// Use simple hash function for reproducibility
	for i := 0; i < 256; i++ {
		fc.gear[i] = uint64(i)*0x9e3779b97f4a7c15 + 0xdeadbeef
	}

	// Calculate masks based on average size
	bits := 0
	size := avgSize
	for size > 1 {
		bits++
		size >>= 1
	}
	fc.maskS = (1 << (bits - 1)) - 1
	fc.maskL = (1 << (bits + 1)) - 1

	return fc
}

// Chunk performs FastCDC chunking on data
func (fc *FastCDC) Chunk(data []byte) ([]ChunkInfo, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var chunks []ChunkInfo
	offset := 0
	index := uint32(0)

	for offset < len(data) {
		remaining := len(data) - offset

		if remaining <= fc.minSize {
			chunks = append(chunks, ChunkInfo{
				Index:            index,
				Offset:           uint64(offset),
				Size:             uint32(remaining),
				UncompressedSize: uint32(remaining),
			})
			break
		}

		cutPoint := fc.findCutPoint(data[offset:], remaining)

		chunks = append(chunks, ChunkInfo{
			Index:            index,
			Offset:           uint64(offset),
			Size:             uint32(cutPoint),
			UncompressedSize: uint32(cutPoint),
		})

		offset += cutPoint
		index++
	}

	return chunks, nil
}

// findCutPoint finds the next cut point using FastCDC
func (fc *FastCDC) findCutPoint(data []byte, remaining int) int {
	normalSize := fc.avgSize
	if normalSize > remaining {
		normalSize = remaining
	}
	if normalSize < fc.minSize {
		normalSize = fc.minSize
	}

	start := fc.minSize
	if start > remaining {
		start = remaining
	}

	end := fc.maxSize
	if end > remaining {
		end = remaining
	}

	var hash uint64 = 0

	// First pass: use maskS for smaller chunks
	for i := start; i < normalSize; i++ {
		hash = (hash << 1) + fc.gear[data[i]]
		if (hash & fc.maskS) == 0 {
			return i
		}
	}

	// Second pass: use maskL for larger chunks
	for i := normalSize; i < end; i++ {
		hash = (hash << 1) + fc.gear[data[i]]
		if (hash & fc.maskL) == 0 {
			return i
		}
	}

	return end
}

// CompressorV2 provides compression with pool support
type CompressorV2 struct {
	algorithm string
	zstdEnc   *sync.Pool
}

// NewCompressorV2 creates a new compressor with pooling
func NewCompressorV2(algorithm string) *CompressorV2 {
	c := &CompressorV2{algorithm: algorithm}

	if algorithm == "zstd" {
		c.zstdEnc = &sync.Pool{
			New: func() interface{} {
				enc, _ := zstd.NewWriter(nil)
				return enc
			},
		}
	}

	return c
}

// Compress compresses data
func (c *CompressorV2) Compress(data []byte) ([]byte, error) {
	switch c.algorithm {
	case "lz4_block":
		return c.compressLZ4(data)
	case "gzip":
		return c.compressGzip(data)
	case "zstd":
		return c.compressZstd(data)
	case "none":
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported compressor: %s", c.algorithm)
	}
}

func (c *CompressorV2) compressLZ4(data []byte) ([]byte, error) {
	return data, nil
}

func (c *CompressorV2) compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *CompressorV2) compressZstd(data []byte) ([]byte, error) {
	if c.zstdEnc != nil {
		enc := c.zstdEnc.Get().(*zstd.Encoder)
		defer c.zstdEnc.Put(enc)
		return enc.EncodeAll(data, nil), nil
	}
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	defer enc.Close()
	return enc.EncodeAll(data, nil), nil
}

// Algorithm returns the compression algorithm
func (c *CompressorV2) Algorithm() string {
	return c.algorithm
}

// ChunkResult represents the result of chunking and compression
type ChunkResult struct {
	Chunks           []ChunkInfo
	CompressedBlob   []byte
	BlobDigest       digest.Digest
	UncompressedSize int64
	CompressedSize   int64
}

// StreamingChunker performs chunking and compression in a pipeline
type StreamingChunker struct {
	fastCDC    *FastCDC
	compressor *CompressorV2
	chunkSize  int
}

// NewStreamingChunker creates a new streaming chunker
func NewStreamingChunker(minSize, avgSize, maxSize int, algorithm string) *StreamingChunker {
	return &StreamingChunker{
		fastCDC:    NewFastCDC(minSize, avgSize, maxSize),
		compressor: NewCompressorV2(algorithm),
		chunkSize:  avgSize,
	}
}

// Process processes a tar layer and returns the chunk result
func (sc *StreamingChunker) Process(ctx context.Context, data []byte) (*ChunkResult, error) {
	if len(data) == 0 {
		return &ChunkResult{
			Chunks:         nil,
			CompressedBlob: nil,
			BlobDigest:     digest.FromBytes(nil),
		}, nil
	}

	chunks, err := sc.fastCDC.Chunk(data)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk data: %w", err)
	}

	var blob []byte
	var compressedChunks []ChunkInfo
	offset := int64(0)

	for i, chunk := range chunks {
		chunkData := data[chunk.Offset : chunk.Offset+uint64(chunk.Size)]
		compressed, err := sc.compressor.Compress(chunkData)
		if err != nil {
			return nil, fmt.Errorf("failed to compress chunk %d: %w", i, err)
		}

		compressedChunk := ChunkInfo{
			Index:            uint32(i),
			Compressed:       len(compressed) < len(chunkData),
			Offset:           uint64(offset),
			Size:             uint32(len(compressed)),
			UncompressedSize: chunk.Size,
		}
		compressedChunks = append(compressedChunks, compressedChunk)

		blob = append(blob, compressed...)
		offset += int64(len(compressed))
	}

	return &ChunkResult{
		Chunks:           compressedChunks,
		CompressedBlob:   blob,
		BlobDigest:       digest.FromBytes(blob),
		UncompressedSize: int64(len(data)),
		CompressedSize:   int64(len(blob)),
	}, nil
}

// ProcessStream processes data from a reader
func (sc *StreamingChunker) ProcessStream(ctx context.Context, r io.Reader) (*ChunkResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}
	return sc.Process(ctx, data)
}
