package nydus

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/core/leases"
	"github.com/moby/buildkit/exporter"
	"github.com/moby/buildkit/exporter/containerimage"
	"github.com/moby/buildkit/nydus/converter"
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

	// Phase 2: Implement layer conversion
	// Check if we have references to convert
	if len(src.Refs) == 0 {
		return resp, nil, nil, errors.New("no layers to export")
	}

	// Create converter
	conv := converter.NewStreamConverter(
		e.opts.ChunkSize/4,     // min chunk size
		e.opts.ChunkSize,       // avg chunk size
		e.opts.ChunkSize*4,     // max chunk size
		e.opts.Compressor,
		e.opts.Parallelism,
	)

	// Convert each layer
	var convertedLayers []*converter.RAFSPair
	for idx, ref := range src.Refs {
		if ref == nil {
			continue
		}

		// Get the layer result
		// TODO: In real implementation, need to get the actual layer reader from ref
		// For now, this is a placeholder that shows the structure
		_ = idx
		_ = conv

		// layerStream := converter.LayerStream{
		// 	Digest:    ref.GetDescription().Digest,
		// 	Size:      ref.GetDescription().Size,
		// 	Reader:    ref.GetReader(),
		// 	DiffID:    ref.GetDiffID(),
		// 	MediaType: ref.GetDescription().MediaType,
		// }

		// pair, err := conv.Convert(ctx, layerStream)
		// if err != nil {
		// 	return nil, nil, nil, errors.Wrapf(err, "failed to convert layer %d", idx)
		// }
		// convertedLayers = append(convertedLayers, pair)
	}

	// Store conversion results in response
	resp["nydus.layers.converted"] = fmt.Sprintf("%d", len(convertedLayers))

	// Phase 2: Partial implementation - full layer conversion in Phase 3
	return resp, nil, nil, errors.New("nydus exporter: Phase 2 - layer conversion framework ready, full implementation in Phase 3")
}

// Helper function to create Nydus annotations
func createNydusAnnotations(fsVersion, compressor string, blobDigest digest.Digest) map[string]string {
	return map[string]string{
		AnnotationNydusFsVersion:  fsVersion,
		AnnotationNydusCompressor: compressor,
		AnnotationNydusBlob:       blobDigest.String(),
	}
}
