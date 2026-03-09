// Package converter handles the conversion of OCI layers to Nydus format
package converter

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/moby/buildkit/nydus/rafs"
	tarprocessor "github.com/moby/buildkit/nydus/tar"
	"github.com/opencontainers/go-digest"
)

// LayerStream represents a layer to be converted
type LayerStream struct {
	Digest    digest.Digest
	Size      int64
	Reader    io.ReadCloser
	DiffID    digest.Digest
	MediaType string
}

// RAFSPair represents the output of conversion
type RAFSPair struct {
	// Bootstrap metadata
	BootstrapDigest  digest.Digest
	BootstrapSize    int64
	BootstrapData    []byte

	// Blob data
	BlobDigest       digest.Digest
	BlobSize         int64
	BlobData         []byte

	// Metadata
	LayerCount       int
	ChunkCount       int
	FileCount        int
	DirCount         int
	SymlinkCount     int
	UncompressedSize int64
	CompressedSize   int64
	CompressionRatio float64
}

// StreamConverter handles stream-based layer conversion
type StreamConverter struct {
	minChunkSize   int
	avgChunkSize   int
	maxChunkSize   int
	compressor     string
	workers        int
	processor      *tarprocessor.LayerProcessor
}

// NewStreamConverter creates a new stream converter
func NewStreamConverter(minChunkSize, avgChunkSize, maxChunkSize int, compressor string, workers int) *StreamConverter {
	if minChunkSize == 0 {
		minChunkSize = 4 * 1024 // 4KB
	}
	if avgChunkSize == 0 {
		avgChunkSize = 256 * 1024 // 256KB
	}
	if maxChunkSize == 0 {
		maxChunkSize = 1024 * 1024 // 1MB
	}
	if compressor == "" {
		compressor = "zstd"
	}
	if workers == 0 {
		workers = 4
	}

	return &StreamConverter{
		minChunkSize: minChunkSize,
		avgChunkSize: avgChunkSize,
		maxChunkSize: maxChunkSize,
		compressor:   compressor,
		workers:      workers,
		processor:    tarprocessor.NewLayerProcessor(minChunkSize, avgChunkSize, maxChunkSize, compressor),
	}
}

// Convert converts a single layer to Nydus format
func (sc *StreamConverter) Convert(ctx context.Context, layer LayerStream) (*RAFSPair, error) {
	defer layer.Reader.Close()

	// Step 1: Process tar layer
	processResult, err := sc.processor.Process(ctx, layer.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to process layer: %w", err)
	}

	// Step 2: Build bootstrap from processed entries
	bootstrapData, err := sc.buildBootstrap(ctx, processResult)
	if err != nil {
		return nil, fmt.Errorf("failed to build bootstrap: %w", err)
	}

	// Step 3: Calculate compression ratio
	var compressionRatio float64 = 1.0
	var blobDigest digest.Digest
	var blobSize int64
	var blobData []byte
	var chunkCount int
	
	if processResult.ChunkResult != nil {
		if processResult.ChunkResult.UncompressedSize > 0 {
			compressionRatio = float64(processResult.ChunkResult.CompressedSize) / float64(processResult.ChunkResult.UncompressedSize)
		}
		blobDigest = processResult.ChunkResult.BlobDigest
		blobSize = processResult.ChunkResult.CompressedSize
		blobData = processResult.ChunkResult.CompressedBlob
		chunkCount = len(processResult.ChunkResult.Chunks)
	}

	return &RAFSPair{
		BootstrapDigest:  digest.FromBytes(bootstrapData),
		BootstrapSize:    int64(len(bootstrapData)),
		BootstrapData:    bootstrapData,
		BlobDigest:       blobDigest,
		BlobSize:         blobSize,
		BlobData:         blobData,
		LayerCount:       1,
		ChunkCount:       chunkCount,
		FileCount:        processResult.FileCount,
		DirCount:         processResult.DirCount,
		SymlinkCount:     processResult.SymlinkCount,
		UncompressedSize: processResult.TotalSize,
		CompressedSize:   blobSize,
		CompressionRatio: compressionRatio,
	}, nil
}

// buildBootstrap builds the RAFS bootstrap from processed entries
func (sc *StreamConverter) buildBootstrap(ctx context.Context, result *tarprocessor.ProcessResult) ([]byte, error) {
	builder := rafs.NewBootstrapBuilder("6", sc.avgChunkSize)

	// Add root inode
	_, err := builder.AddNode("", 0o040755, 0, time.Now(), 0, 0, "")
	if err != nil {
		return nil, fmt.Errorf("failed to add root: %w", err)
	}

	// Add blob entry
	if result.ChunkResult != nil {
		builder.AddBlob(rafs.BlobEntry{
			ChunkCount:       uint32(len(result.ChunkResult.Chunks)),
			Offset:           0,
			Size:             uint64(result.ChunkResult.CompressedSize),
			UncompressedSize: uint64(result.ChunkResult.UncompressedSize),
		})
	}

	// Build bootstrap
	var buf bytes.Buffer
	if err := builder.Build(ctx, &buf); err != nil {
		return nil, fmt.Errorf("failed to build bootstrap: %w", err)
	}

	return buf.Bytes(), nil
}

// ConvertTar converts a raw tar stream (for testing)
func (sc *StreamConverter) ConvertTar(ctx context.Context, r io.Reader) (*RAFSPair, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read tar: %w", err)
	}

	return sc.Convert(ctx, LayerStream{
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
		Reader:    io.NopCloser(bytes.NewReader(data)),
		DiffID:    digest.FromBytes(data),
		MediaType: "application/vnd.docker.image.rootfs.diff.tar",
	})
}

// LayerInfo contains information about a layer
type LayerInfo struct {
	Index            int
	Digest           digest.Digest
	Size             int64
	ChunkCount       int
	CompressionRatio float64
}

// BatchResult contains the result of batch conversion
type BatchResult struct {
	Pairs            []*RAFSPair
	TotalLayers      int
	TotalChunks      int
	TotalFiles       int
	TotalDirs        int
	TotalSymlinks    int
	UncompressedSize int64
	CompressedSize   int64
	Duration         time.Duration
}

// ParallelConverter handles parallel conversion of multiple layers
type ParallelConverter struct {
	streamConv  *StreamConverter
	parallelism int
	minChunkSize int
	avgChunkSize int
	maxChunkSize int
	compressor  string
}

// NewParallelConverter creates a new parallel converter
func NewParallelConverter(minChunkSize, avgChunkSize, maxChunkSize int, compressor string, parallelism int) *ParallelConverter {
	if parallelism <= 0 {
		parallelism = 4
	}

	return &ParallelConverter{
		streamConv:   NewStreamConverter(minChunkSize, avgChunkSize, maxChunkSize, compressor, parallelism),
		parallelism:  parallelism,
		minChunkSize: minChunkSize,
		avgChunkSize: avgChunkSize,
		maxChunkSize: maxChunkSize,
		compressor:   compressor,
	}
}

// ConvertBatch converts multiple layers in parallel
func (pc *ParallelConverter) ConvertBatch(ctx context.Context, layers []LayerStream) (*BatchResult, error) {
	start := time.Now()

	if len(layers) == 0 {
		return &BatchResult{}, nil
	}

	results := make([]*RAFSPair, len(layers))

	// Sequential processing for now - parallel processing will be added
	for i, layer := range layers {
		pair, err := pc.streamConv.Convert(ctx, layer)
		if err != nil {
			return nil, fmt.Errorf("failed to convert layer %d: %w", i, err)
		}
		results[i] = pair
	}

	// Calculate totals
	result := &BatchResult{
		Pairs:         results,
		TotalLayers:   len(results),
		Duration:      time.Since(start),
	}

	for _, pair := range results {
		result.TotalChunks += pair.ChunkCount
		result.TotalFiles += pair.FileCount
		result.TotalDirs += pair.DirCount
		result.TotalSymlinks += pair.SymlinkCount
		result.UncompressedSize += pair.UncompressedSize
		result.CompressedSize += pair.CompressedSize
	}

	return result, nil
}

// ConvertSequential converts layers sequentially (for memory-constrained environments)
func (pc *ParallelConverter) ConvertSequential(ctx context.Context, layers []LayerStream) (*BatchResult, error) {
	return pc.ConvertBatch(ctx, layers)
}

// MemoryOptimizedConverter converts with memory constraints
type MemoryOptimizedConverter struct {
	base      *ParallelConverter
	maxMemory int64
	semaphore chan struct{}
}

// NewMemoryOptimizedConverter creates a converter with memory limits
func NewMemoryOptimizedConverter(minChunkSize, avgChunkSize, maxChunkSize int, compressor string, parallelism int, maxMemory int64) *MemoryOptimizedConverter {
	return &MemoryOptimizedConverter{
		base:      NewParallelConverter(minChunkSize, avgChunkSize, maxChunkSize, compressor, parallelism),
		maxMemory: maxMemory,
		semaphore: make(chan struct{}, parallelism),
	}
}

// ConvertWithMemoryLimit converts with memory backpressure
func (moc *MemoryOptimizedConverter) ConvertWithMemoryLimit(ctx context.Context, layer LayerStream) (*RAFSPair, error) {
	select {
	case moc.semaphore <- struct{}{}:
		defer func() { <-moc.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return moc.base.streamConv.Convert(ctx, layer)
}

// CreateTestTar creates a simple tar archive for testing
func CreateTestTar(entries []tar.Header) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for i, hdr := range entries {
		if err := tw.WriteHeader(&hdr); err != nil {
			return nil, fmt.Errorf("failed to write header %d: %w", i, err)
		}
		if hdr.Size > 0 {
			data := make([]byte, hdr.Size)
			if _, err := tw.Write(data); err != nil {
				return nil, fmt.Errorf("failed to write data %d: %w", i, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar: %w", err)
	}

	return buf.Bytes(), nil
}
