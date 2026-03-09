// Package tar provides streaming tar processing for Nydus conversion
package tar

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/moby/buildkit/nydus/rafs"
)

// Entry represents a tar entry with its content
type Entry struct {
	Header  *tar.Header
	Content []byte
}

// IsDir returns true if entry is a directory
func (e *Entry) IsDir() bool {
	return e.Header.Typeflag == tar.TypeDir
}

// IsReg returns true if entry is a regular file
func (e *Entry) IsReg() bool {
	return e.Header.Typeflag == tar.TypeReg || e.Header.Typeflag == tar.TypeRegA
}

// IsSymlink returns true if entry is a symlink
func (e *Entry) IsSymlink() bool {
	return e.Header.Typeflag == tar.TypeSymlink
}

// StreamingReader reads tar entries in a streaming fashion
type StreamingReader struct {
	tr     *tar.Reader
	buffer *bufio.Reader
}

// NewStreamingReader creates a new streaming tar reader
func NewStreamingReader(r io.Reader) (*StreamingReader, error) {
	br := bufio.NewReader(r)

	// Detect compression
	magic, err := br.Peek(2)
	if err != nil {
		return nil, fmt.Errorf("failed to peek: %w", err)
	}

	// Check for gzip magic
	if magic[0] == 0x1f && magic[1] == 0x8b {
		gr, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return &StreamingReader{
			tr:     tar.NewReader(gr),
			buffer: nil,
		}, nil
	}

	return &StreamingReader{
		tr:     tar.NewReader(br),
		buffer: br,
	}, nil
}

// Next returns the next tar entry
func (sr *StreamingReader) Next() (*Entry, error) {
	hdr, err := sr.tr.Next()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read tar header: %w", err)
	}

	// Read content
	content, err := io.ReadAll(sr.tr)
	if err != nil {
		return nil, fmt.Errorf("failed to read tar content: %w", err)
	}

	return &Entry{
		Header:  hdr,
		Content: content,
	}, nil
}

// LayerProcessor processes a tar layer and extracts file metadata
type LayerProcessor struct {
	chunker *rafs.StreamingChunker
}

// NewLayerProcessor creates a new layer processor
func NewLayerProcessor(minSize, avgSize, maxSize int, compressor string) *LayerProcessor {
	return &LayerProcessor{
		chunker: rafs.NewStreamingChunker(minSize, avgSize, maxSize, compressor),
	}
}

// ProcessResult contains the result of processing a layer
type ProcessResult struct {
	// File entries extracted from tar
	Entries []FileEntry

	// Chunking result
	ChunkResult *rafs.ChunkResult

	// Statistics
	FileCount    int
	DirCount     int
	SymlinkCount int
	TotalSize    int64
}

// FileEntry represents a file entry in the layer
type FileEntry struct {
	Path     string
	Size     int64
	Mode     os.FileMode
	Uid      int
	Gid      int
	ModTime  time.Time
	Linkname string // For symlinks
	Digest   string
	Chunks   []rafs.ChunkInfo
}

// Process processes a tar layer
func (lp *LayerProcessor) Process(ctx context.Context, r io.Reader) (*ProcessResult, error) {
	sr, err := NewStreamingReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming reader: %w", err)
	}

	result := &ProcessResult{
		Entries: []FileEntry{},
	}

	// Collect all file data for chunking
	var fileData []byte
	var fileEntries []FileEntry

	for {
		entry, err := sr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		switch {
		case entry.IsDir():
			result.DirCount++
			result.Entries = append(result.Entries, FileEntry{
				Path:    entry.Header.Name,
				Mode:    os.FileMode(entry.Header.Mode) | os.ModeDir,
				Uid:     entry.Header.Uid,
				Gid:     entry.Header.Gid,
				ModTime: entry.Header.ModTime,
			})

		case entry.IsSymlink():
			result.SymlinkCount++
			result.Entries = append(result.Entries, FileEntry{
				Path:     entry.Header.Name,
				Mode:     os.FileMode(entry.Header.Mode) | os.ModeSymlink,
				Uid:      entry.Header.Uid,
				Gid:      entry.Header.Gid,
				ModTime:  entry.Header.ModTime,
				Linkname: entry.Header.Linkname,
			})

		case entry.IsReg():
			result.FileCount++
			result.TotalSize += entry.Header.Size

			// Record file entry
			fileEntry := FileEntry{
				Path:    entry.Header.Name,
				Size:    entry.Header.Size,
				Mode:    os.FileMode(entry.Header.Mode),
				Uid:     entry.Header.Uid,
				Gid:     entry.Header.Gid,
				ModTime: entry.Header.ModTime,
			}
			fileEntries = append(fileEntries, fileEntry)

			// Append to file data for chunking
			fileData = append(fileData, entry.Content...)

		default:
			// Skip other types (devices, etc.)
			continue
		}
	}

	// Process file data through chunker
	if len(fileData) > 0 {
		chunkResult, err := lp.chunker.Process(ctx, fileData)
		if err != nil {
			return nil, fmt.Errorf("failed to chunk data: %w", err)
		}

		result.ChunkResult = chunkResult

		// Assign chunks to file entries
		// This is a simplified version - in production, need to track file boundaries
		offset := uint64(0)
		for i := range fileEntries {
			if offset < uint64(len(fileData)) {
				// Find chunks that belong to this file
				var fileChunks []rafs.ChunkInfo
				for _, chunk := range chunkResult.Chunks {
					if chunk.Offset >= offset && chunk.Offset < offset+uint64(fileEntries[i].Size) {
						fileChunks = append(fileChunks, chunk)
					}
				}
				fileEntries[i].Chunks = fileChunks
				offset += uint64(fileEntries[i].Size)
			}
		}
	}

	result.Entries = append(result.Entries, fileEntries...)

	return result, nil
}

// Pipeline processes multiple layers in a pipeline
type Pipeline struct {
	processors []*LayerProcessor
	workers    int
}

// NewPipeline creates a new processing pipeline
func NewPipeline(minSize, avgSize, maxSize int, compressor string, workers int) *Pipeline {
	return &Pipeline{
		processors: make([]*LayerProcessor, workers),
		workers:    workers,
	}
}

// ProcessBatch processes multiple layers
func (p *Pipeline) ProcessBatch(ctx context.Context, readers []io.Reader) ([]*ProcessResult, error) {
	results := make([]*ProcessResult, len(readers))

	// Simple sequential processing for now
	// TODO: Add parallel processing with worker pool
	for i, r := range readers {
		processor := NewLayerProcessor(4*1024, 256*1024, 1024*1024, "zstd")
		result, err := processor.Process(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("failed to process layer %d: %w", i, err)
		}
		results[i] = result
	}

	return results, nil
}

// EstimateChunkCount estimates the number of chunks for a given size
func EstimateChunkCount(size int64, avgChunkSize int) int {
	if size == 0 {
		return 0
	}
	count := int(size) / avgChunkSize
	if int(size)%avgChunkSize > 0 {
		count++
	}
	return count
}

// CalculateOptimalChunkSize calculates optimal chunk size based on file size
func CalculateOptimalChunkSize(fileSize int64) int {
	switch {
	case fileSize < 1024*1024: // < 1MB
		return 64 * 1024 // 64KB
	case fileSize < 10*1024*1024: // < 10MB
		return 256 * 1024 // 256KB
	case fileSize < 100*1024*1024: // < 100MB
		return 512 * 1024 // 512KB
	default:
		return 1024 * 1024 // 1MB
	}
}
