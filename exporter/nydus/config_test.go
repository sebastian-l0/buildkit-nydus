package nydus

import (
	"testing"

	"github.com/opencontainers/go-digest"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestConfigBuilder_BuildConfig(t *testing.T) {
	builder := NewConfigBuilder()

	tests := []struct {
		name         string
		architecture string
		os           string
		layerCount   int
		wantErr      bool
	}{
		{
			name:         "default values",
			architecture: "",
			os:           "",
			layerCount:   3,
			wantErr:      false,
		},
		{
			name:         "custom values",
			architecture: "arm64",
			os:           "linux",
			layerCount:   5,
			wantErr:      false,
		},
		{
			name:         "zero layers",
			architecture: "amd64",
			os:           "linux",
			layerCount:   0,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := builder.BuildConfig(tt.architecture, tt.os, tt.layerCount)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)

			// Check architecture and OS
			if tt.architecture != "" {
				require.Equal(t, tt.architecture, config.Platform.Architecture)
			}
			if tt.os != "" {
				require.Equal(t, tt.os, config.Platform.OS)
			}

			// Check rootfs
			require.Equal(t, "layers", config.RootFS.Type)
			require.Equal(t, tt.layerCount, len(config.RootFS.DiffIDs))

			// Check created time is set
			require.NotNil(t, config.Created)
		})
	}
}

func TestConfigBuilder_BuildConfigFromBase(t *testing.T) {
	builder := NewConfigBuilder()

	base := &ocispecs.Image{
		Platform: ocispecs.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
		Author: "test-author",
		Config: ocispecs.ImageConfig{
			Env:        []string{"PATH=/usr/bin"},
			WorkingDir: "/app",
		},
		RootFS: ocispecs.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{digest.FromString("layer1")},
		},
	}

	tests := []struct {
		name       string
		base       *ocispecs.Image
		layerCount int
		wantErr    bool
	}{
		{
			name:       "valid base",
			base:       base,
			layerCount: 3,
			wantErr:    false,
		},
		{
			name:       "nil base",
			base:       nil,
			layerCount: 1,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := builder.BuildConfigFromBase(tt.base, tt.layerCount)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)

			// Check base fields are preserved
			require.Equal(t, tt.base.Platform.Architecture, config.Platform.Architecture)
			require.Equal(t, tt.base.Platform.OS, config.Platform.OS)
			require.Equal(t, tt.base.Author, config.Author)
			require.Equal(t, tt.base.Config.Env, config.Config.Env)
			require.Equal(t, tt.base.Config.WorkingDir, config.Config.WorkingDir)

			// Check layer count is updated
			require.Equal(t, tt.layerCount, len(config.RootFS.DiffIDs))

			// Check created time is updated
			require.NotNil(t, config.Created)
		})
	}
}

func TestConfigToDescriptor(t *testing.T) {
	config := &ocispecs.Image{
		Platform: ocispecs.Platform{
			Architecture: "amd64",
			OS:           "linux",
		},
		RootFS: ocispecs.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{},
		},
	}

	desc, data, err := ConfigToDescriptor(config)

	require.NoError(t, err)
	require.NotNil(t, desc)
	require.NotNil(t, data)

	// Check descriptor fields
	require.Equal(t, ocispecs.MediaTypeImageConfig, desc.MediaType)
	require.NotEmpty(t, desc.Digest)
	require.Greater(t, desc.Size, int64(0))

	// Verify digest matches data
	expectedDigest := digest.FromBytes(data)
	require.Equal(t, expectedDigest, desc.Digest)
}
