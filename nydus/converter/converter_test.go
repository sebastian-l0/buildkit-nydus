package converter

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStreamConverter(t *testing.T) {
	tests := []struct {
		name       string
		chunkSize  int
		compressor string
		workers    int
	}{
		{"default values", 0, "", 0},
		{"custom values", 1024, "zstd", 8},
		{"lz4 compressor", 2048, "lz4_block", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewStreamConverter(tt.chunkSize, tt.compressor, tt.workers)
			assert.NotNil(t, sc)
			if tt.chunkSize == 0 {
				assert.Equal(t, 256*1024, sc.chunkSize)
			} else {
				assert.Equal(t, tt.chunkSize, sc.chunkSize)
			}
			if tt.compressor == "" {
				assert.Equal(t, "lz4_block", sc.compressor)
			} else {
				assert.Equal(t, tt.compressor, sc.compressor)
			}
		})
	}
}

func TestStreamConverterConvert(t *testing.T) {
	sc := NewStreamConverter(4096, "none", 1)

	tests := []struct {
		name     string
		data     []byte
		wantErr  bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "small data",
			data:    []byte("hello world"),
			wantErr: false,
		},
		{
			name:    "larger data",
			data:    bytes.Repeat([]byte("x"), 10000),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer := LayerStream{
				Digest:   digest.FromBytes(tt.data),
				Size:     int64(len(tt.data)),
				Reader:   io.NopCloser(bytes.NewReader(tt.data)),
				DiffID:   digest.FromBytes(tt.data),
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
			assert.NotEmpty(t, result.BlobDigest)
			assert.GreaterOrEqual(t, result.ChunkCount, 0)
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
			pc := NewParallelConverter(4096, "none", tt.parallelism)
			assert.NotNil(t, pc)
			assert.NotNil(t, pc.streamConv)
			if tt.parallelism > 0 {
				assert.Equal(t, tt.parallelism, pc.parallelism)
			}
		})
	}
}

func TestParallelConverterConvertBatch(t *testing.T) {
	pc := NewParallelConverter(4096, "none", 2)

	// Create multiple test layers
	layers := []LayerStream{
		{
			Digest:   digest.FromString("layer1"),
			Size:     100,
			Reader:   io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), 100))),
			DiffID:   digest.FromString("layer1"),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		},
		{
			Digest:   digest.FromString("layer2"),
			Size:     200,
			Reader:   io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("b"), 200))),
			DiffID:   digest.FromString("layer2"),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		},
		{
			Digest:   digest.FromString("layer3"),
			Size:     0,
			Reader:   io.NopCloser(bytes.NewReader([]byte{})),
			DiffID:   digest.FromString("layer3"),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		},
	}

	results, err := pc.ConvertBatch(context.Background(), layers)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	for i, result := range results {
		assert.NotNil(t, result, "result %d should not be nil", i)
		if result != nil {
			assert.NotEmpty(t, result.BootstrapDigest)
		}
	}
}

func TestParallelConverterConvertSequential(t *testing.T) {
	pc := NewParallelConverter(4096, "none", 1)

	layers := []LayerStream{
		{
			Digest:   digest.FromString("layer1"),
			Reader:   io.NopCloser(bytes.NewReader([]byte("data1"))),
			DiffID:   digest.FromString("layer1"),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		},
		{
			Digest:   digest.FromString("layer2"),
			Reader:   io.NopCloser(bytes.NewReader([]byte("data2"))),
			DiffID:   digest.FromString("layer2"),
			MediaType: "application/vnd.docker.image.rootfs.diff.tar",
		},
	}

	results, err := pc.ConvertSequential(context.Background(), layers)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestNewMemoryOptimizedConverter(t *testing.T) {
	moc := NewMemoryOptimizedConverter(4096, "none", 2, 1024*1024*1024)
	assert.NotNil(t, moc)
	assert.NotNil(t, moc.base)
	assert.Equal(t, int64(1024*1024*1024), moc.maxMemory)
}

func TestMemoryOptimizedConverterConvertWithMemoryLimit(t *testing.T) {
	moc := NewMemoryOptimizedConverter(4096, "none", 2, 1024*1024*1024)

	data := []byte("test data for conversion")
	layer := LayerStream{
		Digest:   digest.FromBytes(data),
		Size:     int64(len(data)),
		Reader:   io.NopCloser(bytes.NewReader(data)),
		DiffID:   digest.FromBytes(data),
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
