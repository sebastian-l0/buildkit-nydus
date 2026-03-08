package nydus

import (
	"context"
	"strconv"
	"strings"

	"github.com/moby/buildkit/cache/config"
	"github.com/moby/buildkit/util/compression"
	"github.com/pkg/errors"
)

// Opts contains the configuration options for the Nydus exporter
type Opts struct {
	RefCfg config.RefConfig

	// Nydus specific options
	FsVersion      string
	Compressor     string
	ChunkSize      int
	BlobInlineMeta bool

	// Output options
	DestPath string
	Push     bool
	Name     string
}

// Load parses the exporter attributes and populates the Opts struct
func (o *Opts) Load(ctx context.Context, attrs map[string]string) (map[string]string, error) {
	remaining := make(map[string]string)

	for k, v := range attrs {
		switch k {
		case "fs-version":
			if v != "5" && v != "6" {
				return nil, errors.Errorf("invalid fs-version %q, must be 5 or 6", v)
			}
			o.FsVersion = v
		case "compressor":
			validCompressors := []string{"lz4_block", "gzip", "zstd", "none"}
			found := false
			for _, c := range validCompressors {
				if v == c {
					found = true
					break
				}
			}
			if !found {
				return nil, errors.Errorf("invalid compressor %q, must be one of %v", v, validCompressors)
			}
			o.Compressor = v
		case "chunk-size":
			chunkSize, err := strconv.ParseInt(v, 0, 64)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid chunk-size %q", v)
			}
			// Validate chunk size is power of 2 and >= 4096
			if chunkSize < 4096 || (chunkSize&(chunkSize-1)) != 0 {
				return nil, errors.Errorf("invalid chunk-size %q, must be power of 2 and >= 4096", v)
			}
			o.ChunkSize = int(chunkSize)
		case "blob-inline-meta":
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, errors.Wrapf(err, "non-bool value specified for %s", k)
			}
			o.BlobInlineMeta = b
		case "dest":
			o.DestPath = v
		case "push":
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, errors.Wrapf(err, "non-bool value specified for %s", k)
			}
			o.Push = b
		case "name":
			o.Name = v
		default:
			// Handle compression-related options
			if strings.HasPrefix(k, "compression") {
				remaining[k] = v
			} else {
				remaining[k] = v
			}
		}
	}

	// Set defaults
	if o.FsVersion == "" {
		o.FsVersion = "5"
	}
	if o.Compressor == "" {
		o.Compressor = "lz4_block"
	}
	if o.ChunkSize == 0 {
		o.ChunkSize = 0x100000 // 1MB default
	}

	// Parse compression options
	compConfig, err := compression.ParseAttributes(remaining)
	if err != nil {
		return nil, err
	}
	o.RefCfg.Compression = compConfig

	return remaining, nil
}

// Validate checks if the options are valid
func (o *Opts) Validate() error {
	if o.Push && o.Name == "" {
		return errors.New("name is required when push is enabled")
	}
	if !o.Push && o.DestPath == "" {
		return errors.New("dest is required when push is not enabled")
	}
	return nil
}
