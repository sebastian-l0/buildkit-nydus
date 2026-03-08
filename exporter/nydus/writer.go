package nydus

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/moby/buildkit/nydus/converter"
	"github.com/moby/buildkit/nydus/utils"
)

// Writer handles writing Nydus image output
type Writer struct {
	destPath string
	push     bool
	name     string
}

// NewWriter creates a new Nydus writer
func NewWriter(destPath string, push bool, name string) *Writer {
	return &Writer{
		destPath: destPath,
		push:     push,
		name:     name,
	}
}

// WriteOutput writes the converted Nydus image to the destination
func (w *Writer) WriteOutput(ctx context.Context, results []*converter.RAFSPair) error {
	if w.push {
		return w.pushToRegistry(ctx, results)
	}
	return w.writeToDisk(ctx, results)
}

func (w *Writer) writeToDisk(ctx context.Context, results []*converter.RAFSPair) error {
	if err := utils.EnsureDir(w.destPath); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write each layer's bootstrap and blob
	for i, result := range results {
		// Write bootstrap
		bootstrapPath := filepath.Join(w.destPath, fmt.Sprintf("layer-%d-bootstrap", i))
		if err := utils.WriteFileAtomically(bootstrapPath, result.BootstrapData); err != nil {
			return fmt.Errorf("failed to write bootstrap for layer %d: %w", i, err)
		}

		// Write blob
		blobPath := filepath.Join(w.destPath, fmt.Sprintf("layer-%d-blob", i))
		if err := utils.WriteFileAtomically(blobPath, result.BlobData); err != nil {
			return fmt.Errorf("failed to write blob for layer %d: %w", i, err)
		}
	}

	return nil
}

func (w *Writer) pushToRegistry(ctx context.Context, results []*converter.RAFSPair) error {
	// TODO Phase 4: Implement registry push
	return fmt.Errorf("registry push not yet implemented")
}

// WriteManifest writes the Nydus manifest
func (w *Writer) WriteManifest(ctx context.Context, manifest *NydusManifest) error {
	// TODO Phase 4: Implement manifest writing
	return nil
}
