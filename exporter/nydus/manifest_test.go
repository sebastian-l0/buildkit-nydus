package nydus

import (
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

	tests := []struct {
		name        string
		layers      []NydusLayer
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty layers",
			layers:  []NydusLayer{},
			wantErr: true,
		},
		{
			name: "single layer",
			layers: []NydusLayer{
				{
					Digest:        "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Size:          1024,
					BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
					BootstrapPath: "/tmp/bootstrap",
				},
			},
			wantErr: false,
		},
		{
			name: "multiple layers",
			layers: []NydusLayer{
				{
					Digest:        "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Size:          1024,
					BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
					BootstrapPath: "/tmp/bootstrap1",
				},
				{
					Digest:        "sha256:6ae8a75555209fd6c44157c0aed8026e7639d7783f0c9a6f9f6a5c8e7d5e2f1a",
					Size:          2048,
					BlobDigest:    "sha256:7af8b86666310fd6c55268c0aed9137f8743d7784f1c9b6a9f7b6d9e8f6e3a2b",
					BootstrapPath: "/tmp/bootstrap2",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid blob digest",
			layers: []NydusLayer{
				{
					Digest:        "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Size:          1024,
					BlobDigest:    "invalid-digest",
					BootstrapPath: "/tmp/bootstrap",
				},
			},
			wantErr:     true,
			errContains: "invalid blob digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := builder.BuildManifest(configDesc, tt.layers)

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
			require.Equal(t, len(tt.layers)+1, len(manifest.Layers))

			// Check blob layers
			for i, layer := range tt.layers {
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

	layers := []NydusLayer{
		{
			Digest:        "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Size:          1024,
			BlobDigest:    "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
			BootstrapPath: "/tmp/bootstrap",
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
