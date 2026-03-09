package converter

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStreamConverter(t *testing.T) {
	tests := []struct {
		name       string
		minSize    int
		avgSize    int
		maxSize    int
		compressor string
		workers    int
	}{
		{"default values", 0, 0, 0, "", 0},
		{"custom values", 4096, 1024, 2048, "zstd", 8},
		{"lz4 compressor", 4096, 256 * 1024, 1024 * 1024, "lz4_block", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewStreamConverter(tt.minSize, tt.avgSize, tt.maxSize, tt.compressor, tt.workers)
			assert.NotNil(t, sc)
			if tt.minSize == 0 {
				assert.Equal(t, 4*1024, sc.minChunkSize)
			}
			if tt.avgSize == 0 {
				assert.Equal(t, 256*1024, sc.avgChunkSize)
			}
			if tt.compressor == "" {
				assert.Equal(t, "zstd", sc.compressor)
			}
		})
	}
}

func TestStreamConverterConvert(t *testing.T) {
	sc := NewStreamConverter(4096, 4096, 8192, "none", 1)

	tests := []struct {
		name    string
		entries int
		wantErr bool
	}{
		{
			name:    "empty tar",
			entries: 0,
			wantErr: false,
		},
		{
			name:    "single file",
			entries: 1,
			wantErr: false,
		},
		{
			name:    "multiple files",
			entries: 3,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test tar entries
			tarEntries := make([]tar.Header, tt.entries)
			for i := 0; i < tt.entries; i++ {
				tarEntries[i] = tar.Header{
					Name: fmt.Sprintf("file%d.txt", i),
					Size: 100,
					Mode: 0644,
				}
			}

			tarData, err := CreateTestTar(tarEntries)
			require.NoError(t, err)

			layer := LayerStream{
				Digest:    digest.FromBytes(tarData),
				Size:      int64(len(tarData)),
				Reader:    io.NopCloser(bytes.NewReader(tarData)),
				DiffID:    digest.FromBytes(tarData),
				MediaType: "application/vnd.docker.image.rootfs.diff.tar",
			}

			result, err := sc.Convert(context.Background(), layer)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.BootstrapDigest)
		})
	}
}

func TestNewParallelConverter(t *testing.T) {
	tests := []struct {
		name        string
		parallelism int
	}{
		{"default parallelism", 0},
		{"custom parallelism", 4},
		{"single thread", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewParallelConverter(4096, 256*1024, 1024*1024, "none", tt.parallelism)
			assert.NotNil(t, pc)
			assert.NotNil(t, pc.streamConv)
			if tt.parallelism > 0 {
				assert.Equal(t, tt.parallelism, pc.parallelism)
			}
		})
	}
}

func TestParallelConverterConvertBatch(t *testing.T) {
	pc := NewParallelConverter(4096, 4096, 8192, "none", 2)

	// Create test tar data
	createLayer := func(name string, size int64) LayerStream {
		tarData, _ := CreateTestTar([]tar.Header{
			{Name: name, Size: size, Mode: 0644},
		})
		return LayerStream{
			Digest:    digest.FromString(name),
			Size:      int64(len(tarData)),
			Reader:    io.NopCloser(bytes.NewReader(tarData)),
			DiffID:    digest.FromString(name),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		}
	}

	layers := []LayerStream{
		createLayer("layer1", 100),
		createLayer("layer2", 200),
		createLayer("layer3", 0),
	}

	result, err := pc.ConvertBatch(context.Background(), layers)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.TotalLayers)
	assert.Len(t, result.Pairs, 3)
}

func TestParallelConverterConvertSequential(t *testing.T) {
	pc := NewParallelConverter(4096, 4096, 8192, "none", 1)

	createLayer := func(name string) LayerStream {
		tarData, _ := CreateTestTar([]tar.Header{
			{Name: name, Size: 10, Mode: 0644},
		})
		return LayerStream{
			Digest:    digest.FromString(name),
			Reader:    io.NopCloser(bytes.NewReader(tarData)),
			DiffID:    digest.FromString(name),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		}
	}

	layers := []LayerStream{
		createLayer("layer1"),
		createLayer("layer2"),
	}

	result, err := pc.ConvertSequential(context.Background(), layers)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalLayers)
}

func TestNewMemoryOptimizedConverter(t *testing.T) {
	moc := NewMemoryOptimizedConverter(4096, 256*1024, 1024*1024, "none", 2, 1024*1024*1024)
	assert.NotNil(t, moc)
	assert.NotNil(t, moc.base)
	assert.Equal(t, int64(1024*1024*1024), moc.maxMemory)
}

func TestMemoryOptimizedConverterConvertWithMemoryLimit(t *testing.T) {
	moc := NewMemoryOptimizedConverter(4096, 4096, 8192, "none", 2, 1024*1024*1024)

	tarData, _ := CreateTestTar([]tar.Header{
		{Name: "test.txt", Size: 24, Mode: 0644},
	})

	layer := LayerStream{
		Digest:    digest.FromBytes(tarData),
		Size:      int64(len(tarData)),
		Reader:    io.NopCloser(bytes.NewReader(tarData)),
		DiffID:    digest.FromBytes(tarData),
		MediaType: "application/vnd.docker.image.rootfs.diff.tar",
	}

	result, err := moc.ConvertWithMemoryLimit(context.Background(), layer)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCacheManager(t *testing.T) {
	cm := NewCacheManager(nil)
	assert.NotNil(t, cm)

	// Test CacheKey generation
	key := cm.CacheKey(
		digest.FromString("test"),
		"6",
		"zstd",
		4096,
	)
	assert.NotEmpty(t, key)
	assert.Contains(t, key, "nydus-v1")

	// Test Get (should return nil for now)
	ctx := context.Background()
	result, err := cm.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Nil(t, result)

	// Test Store (placeholder)
	err = cm.Store(ctx, "test-key", &CachedResult{
		BootstrapDigest: digest.FromString("bootstrap"),
		BlobDigest:      digest.FromString("blob"),
	})
	require.NoError(t, err)

	// Test Invalidate (placeholder)
	err = cm.Invalidate(ctx, "test-key")
	require.NoError(t, err)
}
