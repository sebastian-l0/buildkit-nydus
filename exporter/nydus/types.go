package nydus

const (
	// ExporterNydus is the name of the nydus exporter
	ExporterNydus = "nydus"

	// Nydus media types
	MediaTypeNydusBlob = "application/vnd.oci.image.layer.nydus.blob.v1"
	MediaTypeNydusBootstrap = "application/vnd.oci.image.layer.v1.tar+gzip"

	// Nydus annotations
	AnnotationNydusBlob = "containerd.io/snapshot/nydus.blob"
	AnnotationNydusBootstrap = "containerd.io/snapshot/nydus.bootstrap"
	AnnotationNydusFsVersion = "containerd.io/snapshot/nydus.fs-version"
	AnnotationNydusCompressor = "containerd.io/snapshot/nydus.compressor"
)

// NydusLayer represents a converted Nydus layer
type NydusLayer struct {
	Digest      string
	Size        int64
	BlobPath    string
	BlobDigest  string
	BootstrapPath string
}

// NydusManifest represents the Nydus image manifest
type NydusManifest struct {
	Version      string
	FsVersion    string
	Compressor   string
	ChunkSize    int
	BlobInlineMeta bool
	Layers       []NydusLayer
	Bootstrap    string
}
