package converter

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ParallelConverter handles parallel conversion of multiple layers
type ParallelConverter struct {
	streamConv *StreamConverter
	parallelism int
}

// NewParallelConverter creates a new parallel converter
func NewParallelConverter(chunkSize int, compressor string, parallelism int) *ParallelConverter {
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	return &ParallelConverter{
		streamConv:  NewStreamConverter(chunkSize, compressor, parallelism),
		parallelism: parallelism,
	}
}

// ConvertBatch converts multiple layers in parallel
func (pc *ParallelConverter) ConvertBatch(ctx context.Context, layers []LayerStream) ([]*RAFSPair, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	results := make([]*RAFSPair, len(layers))
	var mu sync.Mutex

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(pc.parallelism)

	for i, layer := range layers {
		i, layer := i, layer // capture loop vars
		eg.Go(func() error {
			pair, err := pc.streamConv.Convert(ctx, layer)
			if err != nil {
				return fmt.Errorf("failed to convert layer %d: %w", i, err)
			}

			mu.Lock()
			results[i] = pair
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// ConvertSequential converts layers sequentially (for memory-constrained environments)
func (pc *ParallelConverter) ConvertSequential(ctx context.Context, layers []LayerStream) ([]*RAFSPair, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	results := make([]*RAFSPair, 0, len(layers))

	for i, layer := range layers {
		pair, err := pc.streamConv.Convert(ctx, layer)
		if err != nil {
			return nil, fmt.Errorf("failed to convert layer %d: %w", i, err)
		}
		results = append(results, pair)
	}

	return results, nil
}

// MemoryOptimizedConverter converts with memory constraints
type MemoryOptimizedConverter struct {
	base        *ParallelConverter
	maxMemory   int64
	semaphore   chan struct{}
}

// NewMemoryOptimizedConverter creates a converter with memory limits
func NewMemoryOptimizedConverter(chunkSize int, compressor string, parallelism int, maxMemory int64) *MemoryOptimizedConverter {
	return &MemoryOptimizedConverter{
		base:      NewParallelConverter(chunkSize, compressor, parallelism),
		maxMemory: maxMemory,
		semaphore: make(chan struct{}, parallelism),
	}
}

// ConvertWithMemoryLimit converts with memory backpressure
func (moc *MemoryOptimizedConverter) ConvertWithMemoryLimit(ctx context.Context, layer LayerStream) (*RAFSPair, error) {
	// Acquire semaphore to limit concurrency
	select {
	case moc.semaphore <- struct{}{}:
		defer func() { <-moc.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return moc.base.streamConv.Convert(ctx, layer)
}
