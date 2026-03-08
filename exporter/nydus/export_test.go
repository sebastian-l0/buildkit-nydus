package nydus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptsLoad(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		attrs       map[string]string
		wantErr     bool
		errContains string
		checkFunc   func(t *testing.T, opts *Opts)
	}{
		{
			name: "default values",
			attrs: map[string]string{
				"dest": "/tmp/output",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.Equal(t, "5", opts.FsVersion)
				assert.Equal(t, "lz4_block", opts.Compressor)
				assert.Equal(t, 0x100000, opts.ChunkSize)
				assert.False(t, opts.BlobInlineMeta)
				assert.Equal(t, "/tmp/output", opts.DestPath)
			},
		},
		{
			name: "valid fs-version 6",
			attrs: map[string]string{
				"dest":       "/tmp/output",
				"fs-version": "6",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.Equal(t, "6", opts.FsVersion)
			},
		},
		{
			name: "invalid fs-version",
			attrs: map[string]string{
				"dest":       "/tmp/output",
				"fs-version": "7",
			},
			wantErr:     true,
			errContains: "invalid fs-version",
		},
		{
			name: "valid compressor gzip",
			attrs: map[string]string{
				"dest":       "/tmp/output",
				"compressor": "gzip",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.Equal(t, "gzip", opts.Compressor)
			},
		},
		{
			name: "valid compressor zstd",
			attrs: map[string]string{
				"dest":       "/tmp/output",
				"compressor": "zstd",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.Equal(t, "zstd", opts.Compressor)
			},
		},
		{
			name: "invalid compressor",
			attrs: map[string]string{
				"dest":       "/tmp/output",
				"compressor": "invalid",
			},
			wantErr:     true,
			errContains: "invalid compressor",
		},
		{
			name: "valid chunk-size power of 2",
			attrs: map[string]string{
				"dest":      "/tmp/output",
				"chunk-size": "8192",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.Equal(t, 8192, opts.ChunkSize)
			},
		},
		{
			name: "invalid chunk-size not power of 2",
			attrs: map[string]string{
				"dest":      "/tmp/output",
				"chunk-size": "1000",
			},
			wantErr:     true,
			errContains: "must be power of 2",
		},
		{
			name: "invalid chunk-size too small",
			attrs: map[string]string{
				"dest":      "/tmp/output",
				"chunk-size": "1024",
			},
			wantErr:     true,
			errContains: "must be power of 2",
		},
		{
			name: "blob-inline-meta true",
			attrs: map[string]string{
				"dest":             "/tmp/output",
				"blob-inline-meta": "true",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.True(t, opts.BlobInlineMeta)
			},
		},
		{
			name: "push enabled requires name",
			attrs: map[string]string{
				"push": "true",
				"name": "registry.example.com/app:tag",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, opts *Opts) {
				assert.True(t, opts.Push)
				assert.Equal(t, "registry.example.com/app:tag", opts.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &Opts{}
			_, err := opts.Load(ctx, tt.attrs)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, opts)
			}
		})
	}
}

func TestOptsValidate(t *testing.T) {
	tests := []struct {
		name        string
		opts        Opts
		wantErr     bool
		errContains string
	}{
		{
			name: "valid with dest",
			opts: Opts{
				DestPath: "/tmp/output",
			},
			wantErr: false,
		},
		{
			name: "valid with push and name",
			opts: Opts{
				Push: true,
				Name: "registry.example.com/app:tag",
			},
			wantErr: false,
		},
		{
			name: "invalid push without name",
			opts: Opts{
				Push: true,
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name:        "invalid without dest and push",
			opts:        Opts{},
			wantErr:     true,
			errContains: "dest is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestExporterConstants(t *testing.T) {
	assert.Equal(t, "nydus", ExporterNydus)
	assert.Equal(t, "application/vnd.oci.image.layer.nydus.blob.v1", MediaTypeNydusBlob)
	assert.Equal(t, "containerd.io/snapshot/nydus.blob", AnnotationNydusBlob)
	assert.Equal(t, "containerd.io/snapshot/nydus.bootstrap", AnnotationNydusBootstrap)
	assert.Equal(t, "containerd.io/snapshot/nydus.fs-version", AnnotationNydusFsVersion)
}

func TestNydusLayer(t *testing.T) {
	layer := NydusLayer{
		Digest:        "sha256:abc123",
		Size:          1024,
		BlobPath:      "/tmp/blob",
		BlobDigest:    "sha256:blob456",
		BootstrapPath: "/tmp/bootstrap",
	}

	assert.Equal(t, "sha256:abc123", layer.Digest)
	assert.Equal(t, int64(1024), layer.Size)
}
