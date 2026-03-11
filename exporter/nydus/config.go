package nydus

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/opencontainers/go-digest"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

// ConfigBuilder builds OCI image configuration for Nydus images
type ConfigBuilder struct{}

// NewConfigBuilder creates a new config builder
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{}
}

// BuildConfig creates a minimal OCI image config for Nydus
func (b *ConfigBuilder) BuildConfig(
	architecture string,
	osName string,
	layerCount int,
) (*ocispecs.Image, error) {
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	if osName == "" {
		osName = runtime.GOOS
	}

	config := &ocispecs.Image{
		Platform: ocispecs.Platform{
			Architecture: architecture,
			OS:           osName,
		},
		RootFS: ocispecs.RootFS{
			Type:    "layers",
			DiffIDs: make([]digest.Digest, layerCount),
		},
		Config: ocispecs.ImageConfig{},
	}

	// Set created time
	now := time.Now().UTC()
	config.Created = &now

	return config, nil
}

// BuildConfigFromBase creates config based on an existing config
func (b *ConfigBuilder) BuildConfigFromBase(
	base *ocispecs.Image,
	layerCount int,
) (*ocispecs.Image, error) {
	if base == nil {
		return nil, fmt.Errorf("base config is nil")
	}

	// Create a copy with updated layer count
	config := *base
	config.RootFS.DiffIDs = make([]digest.Digest, layerCount)

	// Update created time
	now := time.Now().UTC()
	config.Created = &now

	return &config, nil
}

// ConfigToDescriptor serializes config and returns descriptor
func ConfigToDescriptor(config *ocispecs.Image) (ocispecs.Descriptor, []byte, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return ocispecs.Descriptor{}, nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	desc := ocispecs.Descriptor{
		MediaType: ocispecs.MediaTypeImageConfig,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
	}

	return desc, data, nil
}
