package nydus

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

// ManifestBuilder builds OCI compliant manifests for Nydus images
type ManifestBuilder struct {
	fsVersion  string
	compressor string
}

// NewManifestBuilder creates a new manifest builder
func NewManifestBuilder(fsVersion, compressor string) *ManifestBuilder {
	return &ManifestBuilder{
		fsVersion:  fsVersion,
		compressor: compressor,
	}
}

// BuildManifest builds an OCI manifest from Nydus layers
func (b *ManifestBuilder) BuildManifest(
	configDesc ocispecs.Descriptor,
	layers []NydusLayer,
) (*ocispecs.Manifest, error) {
	if len(layers) == 0 {
		return nil, fmt.Errorf("no layers provided")
	}

	var ociLayers []ocispecs.Descriptor

	// Add blob layers first
	for _, layer := range layers {
		blobDigest, err := digest.Parse(layer.BlobDigest)
		if err != nil {
			return nil, fmt.Errorf("invalid blob digest %q: %w", layer.BlobDigest, err)
		}

		ociLayers = append(ociLayers, ocispecs.Descriptor{
			MediaType: MediaTypeNydusBlob,
			Digest:    blobDigest,
			Size:      layer.Size,
			Annotations: map[string]string{
				AnnotationNydusBlob: layer.BlobDigest,
			},
		})
	}

	// Add bootstrap layer as the last layer
	lastLayer := layers[len(layers)-1]
	bootstrapDigest, err := digest.Parse(lastLayer.Digest)
	if err != nil {
		return nil, fmt.Errorf("invalid bootstrap digest %q: %w", lastLayer.Digest, err)
	}

	// Read bootstrap data to get actual size
	bootstrapData, err := readBootstrapFile(lastLayer.BootstrapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bootstrap: %w", err)
	}

	ociLayers = append(ociLayers, ocispecs.Descriptor{
		MediaType: MediaTypeNydusBootstrap,
		Digest:    bootstrapDigest,
		Size:      int64(len(bootstrapData)),
		Annotations: map[string]string{
			AnnotationNydusBootstrap:   "true",
			AnnotationNydusFsVersion:   b.fsVersion,
			AnnotationNydusCompressor:  b.compressor,
		},
	})

	manifest := &ocispecs.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispecs.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    ociLayers,
		Annotations: map[string]string{
			AnnotationNydusFsVersion: b.fsVersion,
		},
	}

	return manifest, nil
}

// readBootstrapFile reads the bootstrap file content
func readBootstrapFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read bootstrap file %q: %w", path, err)
	}
	return data, nil
}

// ManifestToJSON serializes the manifest to JSON
func ManifestToJSON(manifest *ocispecs.Manifest) ([]byte, error) {
	return json.MarshalIndent(manifest, "", "  ")
}

// CalculateManifestDigest calculates the digest of a manifest
func CalculateManifestDigest(manifest *ocispecs.Manifest) (digest.Digest, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return digest.FromBytes(data), nil
}
