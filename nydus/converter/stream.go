// Package converter handles the conversion of OCI layers to Nydus format
package converter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/moby/buildkit/nydus/rafs"
	"github.com/opencontainers/go-digest"
)

// LayerStream represents a layer to be converted
type LayerStream struct {
	Digest   digest.Digest
	Size     int64
	Reader   io.ReadCloser
	DiffID   digest.Digest
	MediaType string
}

// RAFSPair represents the output of conversion
type RAFSPair struct {
	// Bootstrap metadata
	BootstrapDigest digest.Digest
	BootstrapSize   int64
	BootstrapData   []byte

	// Blob data
	BlobDigest digest.Digest
	BlobSize   int64
	BlobData   []byte

	// Metadata
	LayerCount int
	ChunkCount int
}

// StreamConverter handles stream-based layer conversion
type StreamConverter struct {
	chunkSize  int
	compressor string
	workers    int
}

// NewStreamConverter creates a new stream converter
func NewStreamConverter(chunkSize int, compressor string, workers int) *StreamConverter {
	if chunkSize == 0 {
		chunkSize = 256 * 1024 // 256KB default
	}
	if compressor == "" {
		compressor = "lz4_block"
	}
	if workers == 0 {
		workers = 4
	}

	return &StreamConverter{
		chunkSize:  chunkSize,
		compressor: compressor,
		workers:    workers,
	}
}

// Convert converts a single layer to Nydus format
func (sc *StreamConverter) Convert(ctx context.Context, layer LayerStream) (*RAFSPair, error) {
	defer layer.Reader.Close()

	// Step 1: Read layer data
	data, err := io.ReadAll(layer.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer: %w", err)
	}

	// Step 2: Chunk the data
	chunker := rafs.NewChunker(
		sc.chunkSize/4,     // min size
		sc.chunkSize,       // avg size
		sc.chunkSize*4,     // max size
	)

	chunks, err := chunker.Chunk(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk layer: %w", err)
	}

	// Step 3: Compress chunks
	comp := rafs.NewCompressor(sc.compressor)
	
	var blobData []byte
	var chunkInfos []rafs.ChunkInfo
	var offset uint64

	for i, chunk := range chunks {
		chunkData := data[chunk.Offset : chunk.Offset+uint64(chunk.Size)]
		compressed, err := comp.Compress(chunkData)
		if err != nil {
			return nil, fmt.Errorf("failed to compress chunk %d: %w", i, err)
		}

		chunkInfos = append(chunkInfos, rafs.ChunkInfo{
			Index:            uint32(i),
			Compressed:       len(compressed) < len(chunkData),
			Offset:           offset,
			Size:             uint32(len(compressed)),
			UncompressedSize: chunk.Size,
		})

		blobData = append(blobData, compressed...)
		offset += uint64(len(compressed))
	}

	// Step 4: Build bootstrap
	builder := rafs.NewBootstrapBuilder("6", sc.chunkSize)
	
	// Add root inode
	root, err := builder.AddNode("", 0o040755, 0, time.Now(), 0, 0, "")
	if err != nil {
		return nil, fmt.Errorf("failed to add root: %w", err)
	}

	// Add chunks to inode
	for _, chunk := range chunkInfos {
		builder.AddChunk(root, chunk)
	}

	// Add blob entry
	builder.AddBlob(rafs.BlobEntry{
		ChunkCount:       uint32(len(chunkInfos)),
		Offset:           0,
		Size:             uint64(len(blobData)),
		UncompressedSize: uint64(len(data)),
	})

	// Build bootstrap
	bootstrapData, err := sc.buildBootstrap(ctx, builder)
	if err != nil {
		return nil, fmt.Errorf("failed to build bootstrap: %w", err)
	}

	return &RAFSPair{
		BootstrapDigest: digest.FromBytes(bootstrapData),
		BootstrapSize:   int64(len(bootstrapData)),
		BootstrapData:   bootstrapData,
		BlobDigest:      digest.FromBytes(blobData),
		BlobSize:        int64(len(blobData)),
		BlobData:        blobData,
		LayerCount:      1,
		ChunkCount:      len(chunkInfos),
	}, nil
}

func (sc *StreamConverter) buildBootstrap(ctx context.Context, builder *rafs.BootstrapBuilder) ([]byte, error) {
	// Use a pipe to capture bootstrap output
	pr, pw := io.Pipe()
	
	var bootstrapData []byte
	errChan := make(chan error, 1)

	go func() {
		data, err := io.ReadAll(pr)
		if err != nil {
			errChan <- err
			return
		}
		bootstrapData = data
		errChan <- nil
	}()

	if err := builder.Build(ctx, pw); err != nil {
		pw.Close()
		return nil, err
	}
	
	if err := pw.Close(); err != nil {
		return nil, err
	}

	if err := <-errChan; err != nil {
		return nil, err
	}

	return bootstrapData, nil
}
