package utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test", "nested", "dir")

	err := EnsureDir(testDir)
	require.NoError(t, err)
	assert.DirExists(t, testDir)

	// Test idempotency
	err = EnsureDir(testDir)
	require.NoError(t, err)
}

func TestTempDir(t *testing.T) {
	dir, err := TempDir("test")
	require.NoError(t, err)
	assert.DirExists(t, dir)
	defer CleanDir(dir)

	// Check prefix
	assert.Contains(t, filepath.Base(dir), "nydus-test-")
}

func TestCleanDir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "to-clean")
	err := os.Mkdir(testDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(testDir, "file.txt"), []byte("test"), 0644)
	require.NoError(t, err)

	err = CleanDir(testDir)
	require.NoError(t, err)
	assert.NoDirExists(t, testDir)
}

func TestWriteFileAtomically(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	data := []byte("hello world")

	err := WriteFileAtomically(testFile, data)
	require.NoError(t, err)
	assert.FileExists(t, testFile)

	readData, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, data, readData)

	// Test creating nested directory
	nestedFile := filepath.Join(tmpDir, "nested", "dir", "file.txt")
	err = WriteFileAtomically(nestedFile, data)
	require.NoError(t, err)
	assert.FileExists(t, nestedFile)
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")
	data := []byte("copy me")

	err := os.WriteFile(src, data, 0644)
	require.NoError(t, err)

	err = CopyFile(src, dst)
	require.NoError(t, err)
	assert.FileExists(t, dst)

	readData, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, data, readData)
}

func TestWriteFileFromReader(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "reader.txt")
	data := []byte("from reader")
	reader := bytes.NewReader(data)

	err := WriteFileFromReader(testFile, reader)
	require.NoError(t, err)
	assert.FileExists(t, testFile)

	readData, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, data, readData)
}

func TestTarReaderReadEntries(t *testing.T) {
	// Create a tar archive
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add a file
	hdr := &tar.Header{
		Name: "test.txt",
		Size: 5,
		Mode: 0644,
	}
	err := tw.WriteHeader(hdr)
	require.NoError(t, err)
	_, err = tw.Write([]byte("hello"))
	require.NoError(t, err)

	// Add a directory
	hdr2 := &tar.Header{
		Name:     "testdir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	err = tw.WriteHeader(hdr2)
	require.NoError(t, err)

	err = tw.Close()
	require.NoError(t, err)

	// Read entries
	tr := NewTarReader(&buf)
	entries, err := tr.ReadEntries(context.Background())
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "test.txt", entries[0].Name)
	assert.Equal(t, "testdir/", entries[1].Name)
}

func TestTarReaderReadAll(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	content := []byte("file content")
	hdr := &tar.Header{
		Name: "test.txt",
		Size: int64(len(content)),
		Mode: 0644,
	}
	err := tw.WriteHeader(hdr)
	require.NoError(t, err)
	_, err = tw.Write(content)
	require.NoError(t, err)
	err = tw.Close()
	require.NoError(t, err)

	tr := NewTarReader(&buf)
	entries, err := tr.ReadAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "test.txt", entries[0].Header.Name)
	assert.Equal(t, content, entries[0].Content)
}

func TestDecompressTar(t *testing.T) {
	t.Run("plain tar", func(t *testing.T) {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		tw.WriteHeader(&tar.Header{Name: "test.txt", Size: 5, Mode: 0644})
		tw.Write([]byte("hello"))
		tw.Close()

		reader, err := DecompressTar(&buf)
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})

	t.Run("gzipped tar", func(t *testing.T) {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		tw.WriteHeader(&tar.Header{Name: "test.txt", Size: 5, Mode: 0644})
		tw.Write([]byte("hello"))
		tw.Close()
		gw.Close()

		reader, err := DecompressTar(&buf)
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
}
