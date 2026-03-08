package nydus

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/core/leases"
	"github.com/moby/buildkit/exporter"
	"github.com/moby/buildkit/exporter/containerimage"
	"github.com/moby/buildkit/session"
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

	// Phase 1: Basic structure complete
	// TODO Phase 2: Implement layer conversion using converter.StreamConverter
	// TODO Phase 3: Integrate with BuildKit cache
	// TODO Phase 4: Complete manifest generation and output

	return resp, nil, nil, errors.New("nydus exporter: Phase 1 complete - basic framework ready, layer conversion in Phase 2")
}
