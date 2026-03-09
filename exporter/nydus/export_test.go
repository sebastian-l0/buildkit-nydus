package nydus

import (
	"context"
	"testing"

	"github.com/moby/buildkit/exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExporter(t *testing.T) {
	opt := Opt{
		SessionManager: nil,
		ImageWriter:    nil,
		LeaseManager:   nil,
	}

	exp, err := New(opt)
	require.NoError(t, err)
	assert.NotNil(t, exp)
}

func TestNydusExporterResolve(t *testing.T) {
	opt := Opt{}
	exp, err := New(opt)
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name        string
		attrs       map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid with dest",
			attrs: map[string]string{
				"dest": "/tmp/output",
			},
			wantErr: false,
		},
		{
			name: "valid with push and name",
			attrs: map[string]string{
				"push": "true",
				"name": "registry.example.com/app:tag",
			},
			wantErr: false,
		},
		{
			name: "invalid - push without name",
			attrs: map[string]string{
				"push": "true",
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name:        "invalid - no dest and no push",
			attrs:       map[string]string{},
			wantErr:     true,
			errContains: "dest is required",
		},
		{
			name: "with all nydus options",
			attrs: map[string]string{
				"dest":             "/tmp/output",
				"fs-version":       "6",
				"compressor":       "zstd",
				"chunk-size":       "4194304",
				"blob-inline-meta": "true",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := exp.Resolve(ctx, 1, tt.attrs)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, instance)
			assert.Equal(t, 1, instance.ID())
			assert.Equal(t, "nydus", instance.Type())
			assert.Equal(t, "exporting to Nydus image format", instance.Name())
		})
	}
}

func TestNydusExporterInstanceMethods(t *testing.T) {
	opt := Opt{}
	exp, _ := New(opt)
	ctx := context.Background()

	attrs := map[string]string{
		"dest":       "/tmp/output",
		"fs-version": "6",
		"compressor": "zstd",
	}
	instance, err := exp.Resolve(ctx, 1, attrs)
	require.NoError(t, err)

	// Test ID
	assert.Equal(t, 1, instance.ID())

	// Test Type
	assert.Equal(t, "nydus", instance.Type())

	// Test Name
	assert.Equal(t, "exporting to Nydus image format", instance.Name())

	// Test Attrs
	assert.Equal(t, attrs, instance.Attrs())

	// Test Config
	config := instance.Config()
	assert.NotNil(t, config)
}

func TestNydusExporterExport(t *testing.T) {
	opt := Opt{}
	exp, _ := New(opt)
	ctx := context.Background()

	tests := []struct {
		name     string
		attrs    map[string]string
		push     bool
		destPath string
	}{
		{
			name: "export with dest",
			attrs: map[string]string{
				"dest":       "/tmp/nydus-output",
				"fs-version": "5",
				"compressor": "lz4_block",
			},
			destPath: "/tmp/nydus-output",
		},
		{
			name: "export with push",
			attrs: map[string]string{
				"push":       "true",
				"name":       "registry.example.com/test:latest",
				"fs-version": "6",
				"compressor": "gzip",
			},
			push: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := exp.Resolve(ctx, 1, tt.attrs)
			require.NoError(t, err)

			src := &exporter.Source{}
			buildInfo := exporter.ExportBuildInfo{}

			resp, finalize, descRef, err := instance.Export(ctx, src, buildInfo)

			// Currently returns error for Phase 2
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no layers to export")
			assert.NotNil(t, resp)
			assert.Nil(t, finalize)
			assert.Nil(t, descRef)

			// Verify response contains expected fields
			assert.Equal(t, "nydus", resp["exporter.type"])
			assert.NotEmpty(t, resp["nydus.fs-version"])
			assert.NotEmpty(t, resp["nydus.compressor"])
			assert.NotEmpty(t, resp["nydus.chunk-size"])

			if tt.push {
				assert.Equal(t, "true", resp["push"])
				assert.Equal(t, tt.attrs["name"], resp["name"])
			} else {
				assert.Equal(t, tt.destPath, resp["dest"])
			}
		})
	}
}
