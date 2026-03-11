package nydus

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/core/leases"
	"github.com/moby/buildkit/exporter"
	"github.com/moby/buildkit/exporter/containerimage"
	"github.com/moby/buildkit/session"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Opt contains the dependencies required by the Nydus exporter
type Opt struct {
	SessionManager *session.Manager
	ImageWriter    *containerimage.ImageWriter
	LeaseManager   leases.Manager
}

type nydusExporter struct {
	opt Opt
}

type nydusExporterInstance struct {
	*nydusExporter
	id     int
	attrs  map[string]string
	opts   Opts
}

// New creates a new Nydus exporter
func New(opt Opt) (exporter.Exporter, error) {
	return &nydusExporter{opt: opt}, nil
}

func (e *nydusExporter) Resolve(ctx context.Context, id int, attrs map[string]string) (exporter.ExporterInstance, error) {
	i := &nydusExporterInstance{
		nydusExporter: e,
		id:            id,
		attrs:         attrs,
		opts: Opts{
			// Default values will be set in Load()
		},
	}

	_, err := i.opts.Load(ctx, attrs)
	if err != nil {
		return nil, err
	}

	if err := i.opts.Validate(); err != nil {
		return nil, err
	}

	return i, nil
}

func (e *nydusExporterInstance) ID() int {
	return e.id
}

func (e *nydusExporterInstance) Name() string {
	return "exporting to Nydus image format"
}

func (e *nydusExporterInstance) Type() string {
	return ExporterNydus
}

func (e *nydusExporterInstance) Attrs() map[string]string {
	return e.attrs
}

func (e *nydusExporterInstance) Config() *exporter.Config {
	return exporter.NewConfigWithCompression(e.opts.RefCfg.Compression)
}

// ConvertedLayer represents a layer converted to Nydus format
type ConvertedLayer struct {
	NydusLayer
}

func (e *nydusExporterInstance) Export(ctx context.Context, src *exporter.Source, buildInfo exporter.ExportBuildInfo) (map[string]string, exporter.FinalizeFunc, exporter.DescriptorReference, error) {
	resp := make(map[string]string)
	resp["exporter.type"] = ExporterNydus
	resp["nydus.fs-version"] = e.opts.FsVersion
	resp["nydus.compressor"] = e.opts.Compressor
	resp["nydus.chunk-size"] = fmt.Sprintf("%d", e.opts.ChunkSize)

	if e.opts.Push {
		resp["push"] = "true"
		resp["name"] = e.opts.Name
	} else {
		resp["dest"] = e.opts.DestPath
	}

	// Phase 3: Build config and manifest
	refCount := len(src.Refs)
	if src.Ref != nil {
		refCount++
	}
	if refCount == 0 {
		return resp, nil, nil, errors.New("no layers to export")
	}

	// Create config builder and build OCI config
	configBuilder := NewConfigBuilder()
	config, err := configBuilder.BuildConfig("", "", refCount)
	if err != nil {
		return resp, nil, nil, errors.Wrap(err, "failed to build image config")
	}

	// Get config descriptor
	configDesc, configData, err := ConfigToDescriptor(config)
	if err != nil {
		return resp, nil, nil, errors.Wrap(err, "failed to serialize config")
	}

	resp["config.digest"] = configDesc.Digest.String()
	resp["config.size"] = fmt.Sprintf("%d", configDesc.Size)

	// Create manifest builder
	manifestBuilder := NewManifestBuilder(e.opts.FsVersion, e.opts.Compressor)

	// Build sample layers for demonstration (Phase 3: placeholder layers)
	layers := []NydusLayer{
		{
			Digest:        configDesc.Digest.String(),
			Size:          configDesc.Size,
			BlobDigest:    digest.FromBytes(configData).String(),
			BootstrapPath: "/tmp/bootstrap", // Placeholder
		},
	}

	// Build manifest
	manifest, err := manifestBuilder.BuildManifest(configDesc, layers)
	if err != nil {
		return resp, nil, nil, errors.Wrap(err, "failed to build manifest")
	}

	// Serialize manifest
	manifestJSON, err := ManifestToJSON(manifest)
	if err != nil {
		return resp, nil, nil, errors.Wrap(err, "failed to serialize manifest")
	}

	manifestDigest, err := CalculateManifestDigest(manifest)
	if err != nil {
		return resp, nil, nil, errors.Wrap(err, "failed to calculate manifest digest")
	}

	resp["manifest.digest"] = manifestDigest.String()
	resp["manifest.size"] = fmt.Sprintf("%d", len(manifestJSON))
	resp["nydus.layers.count"] = fmt.Sprintf("%d", len(manifest.Layers))

	// Phase 3: Config and manifest generation complete
	return resp, nil, nil, errors.New("nydus exporter: Phase 3 - Config and manifest generation complete. Full layer conversion integration in Phase 4")
}

// Helper function to create Nydus annotations
func createNydusAnnotations(fsVersion, compressor string, blobDigest digest.Digest) map[string]string {
	return map[string]string{
		AnnotationNydusFsVersion:  fsVersion,
		AnnotationNydusCompressor: compressor,
		AnnotationNydusBlob:       blobDigest.String(),
	}
}
