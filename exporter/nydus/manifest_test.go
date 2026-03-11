package nydus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestManifestBuilder_BuildManifest(t *testing.T) {
	builder := NewManifestBuilder("5", "lz4_block")

	configDesc := ocispecs.Descriptor{
		MediaType: ocispecs.MediaTypeImageConfig,
		Digest:    digest.FromString("config"),
		Size:      100,
	}

	// Create temp directory for bootstrap files
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		setupLayers func() ([]NydusLayer, func())
		wantErr     bool
		errContains string
	}{
		{
			name: "empty layers",
			setupLayers: func() ([]NydusLayer, func()) {
				return []NydusLayer{}, func() {}
			},
			wantErr: true,
		},
		{
			name: "single layer",
			setupLayers: func() ([]NydusLayer, func()) {
				bootstrapPath := filepath.Join(tempDir, "bootstrap1")
				err := os.WriteFile(bootstrapPath, []byte("bootstrap data 1"), 0644)
				require.NoError(t, err)
				return []NydusLayer{
					{
						Digest:        digest.FromString("bootstrap data 1").String(),
						Size:          1024,
						BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
						BootstrapPath: bootstrapPath,
					},
				}, func() {}
			},
			wantErr: false,
		},
		{
			name: "multiple layers",
			setupLayers: func() ([]NydusLayer, func()) {
				bootstrapPath1 := filepath.Join(tempDir, "bootstrap1")
				bootstrapPath2 := filepath.Join(tempDir, "bootstrap2")
				err := os.WriteFile(bootstrapPath1, []byte("bootstrap data 1"), 0644)
				require.NoError(t, err)
				err = os.WriteFile(bootstrapPath2, []byte("bootstrap data 2 longer"), 0644)
				require.NoError(t, err)
				return []NydusLayer{
					{
						Digest:        digest.FromString("bootstrap data 1").String(),
						Size:          1024,
						BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
						BootstrapPath: bootstrapPath1,
					},
					{
						Digest:        digest.FromString("bootstrap data 2 longer").String(),
						Size:          2048,
						BlobDigest:    "sha256:7af8b86666310fd6c55268c0aed9137f8743d7784f1c9b6a9f7b6d9e8f6e3a2b",
						BootstrapPath: bootstrapPath2,
					},
				}, func() {}
			},
			wantErr: false,
		},
		{
			name: "invalid blob digest",
			setupLayers: func() ([]NydusLayer, func()) {
				bootstrapPath := filepath.Join(tempDir, "bootstrap_invalid")
				err := os.WriteFile(bootstrapPath, []byte("data"), 0644)
				require.NoError(t, err)
				return []NydusLayer{
					{
						Digest:        "sha256:" + digest.FromString("data").String(),
						Size:          1024,
						BlobDigest:    "invalid-digest",
						BootstrapPath: bootstrapPath,
					},
				}, func() {}
			},
			wantErr:     true,
			errContains: "invalid blob digest",
		},
		{
			name: "missing bootstrap file",
			setupLayers: func() ([]NydusLayer, func()) {
				return []NydusLayer{
					{
						Digest:        "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
						Size:          1024,
						BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
						BootstrapPath: "/nonexistent/path/bootstrap",
					},
				}, func() {}
			},
			wantErr:     true,
			errContains: "failed to read bootstrap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layers, cleanup := tt.setupLayers()
			defer cleanup()

			manifest, err := builder.BuildManifest(configDesc, layers)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, manifest)
			require.Equal(t, 2, manifest.SchemaVersion)
			require.Equal(t, ocispecs.MediaTypeImageManifest, manifest.MediaType)
			require.Equal(t, configDesc, manifest.Config)
			require.NotEmpty(t, manifest.Annotations)
			require.Equal(t, "5", manifest.Annotations[AnnotationNydusFsVersion])

			// Check layers: n blobs + 1 bootstrap
			require.Equal(t, len(layers)+1, len(manifest.Layers))

			// Check blob layers
			for i, layer := range layers {
				require.Equal(t, MediaTypeNydusBlob, manifest.Layers[i].MediaType)
				require.Equal(t, layer.BlobDigest, manifest.Layers[i].Annotations[AnnotationNydusBlob])
				require.Equal(t, layer.Size, manifest.Layers[i].Size)
			}

			// Check bootstrap layer (last one)
			bootstrapLayer := manifest.Layers[len(manifest.Layers)-1]
			require.Equal(t, MediaTypeNydusBootstrap, bootstrapLayer.MediaType)
			require.Equal(t, "true", bootstrapLayer.Annotations[AnnotationNydusBootstrap])
			require.Equal(t, "5", bootstrapLayer.Annotations[AnnotationNydusFsVersion])
			require.Equal(t, "lz4_block", bootstrapLayer.Annotations[AnnotationNydusCompressor])
		})
	}
}

func TestManifestBuilder_BuildManifest_ValidatesAnnotations(t *testing.T) {
	builder := NewManifestBuilder("6", "zstd")

	configDesc := ocispecs.Descriptor{
		MediaType: ocispecs.MediaTypeImageConfig,
		Digest:    digest.FromString("config"),
		Size:      100,
	}

	tempDir := t.TempDir()
	bootstrapPath := filepath.Join(tempDir, "bootstrap")
	err := os.WriteFile(bootstrapPath, []byte("bootstrap data"), 0644)
	require.NoError(t, err)

	layers := []NydusLayer{
		{
			Digest:        digest.FromString("bootstrap data").String(),
			Size:          1024,
			BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
			BootstrapPath: bootstrapPath,
		},
	}

	manifest, err := builder.BuildManifest(configDesc, layers)
	require.NoError(t, err)

	// Verify annotations are set correctly
	require.Equal(t, "6", manifest.Annotations[AnnotationNydusFsVersion])
	
	bootstrapLayer := manifest.Layers[len(manifest.Layers)-1]
	require.Equal(t, "6", bootstrapLayer.Annotations[AnnotationNydusFsVersion])
	require.Equal(t, "zstd", bootstrapLayer.Annotations[AnnotationNydusCompressor])
}

func TestManifestToJSON(t *testing.T) {
	manifest := &ocispecs.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispecs.MediaTypeImageManifest,
		Config: ocispecs.Descriptor{
			MediaType: ocispecs.MediaTypeImageConfig,
			Digest:    digest.FromString("test"),
			Size:      100,
		},
		Layers: []ocispecs.Descriptor{
			{
				MediaType: MediaTypeNydusBlob,
				Digest:    digest.FromString("blob"),
				Size:      1024,
			},
		},
	}

	data, err := ManifestToJSON(manifest)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.Contains(t, string(data), "schemaVersion")
	require.Contains(t, string(data), "mediaType")
}

func TestCalculateManifestDigest(t *testing.T) {
	manifest := &ocispecs.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispecs.MediaTypeImageManifest,
		Config: ocispecs.Descriptor{
			MediaType: ocispecs.MediaTypeImageConfig,
			Digest:    digest.FromString("test"),
			Size:      100,
		},
		Layers: []ocispecs.Descriptor{},
	}

	dig, err := CalculateManifestDigest(manifest)
	require.NoError(t, err)
	require.NotEmpty(t, dig)
	require.True(t, dig.Validate() == nil)
}
