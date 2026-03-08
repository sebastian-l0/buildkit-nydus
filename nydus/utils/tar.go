// Package utils provides utility functions for Nydus conversion
package utils

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
)

// TarReader provides utilities for reading tar streams
type TarReader struct {
	r io.Reader
}

// NewTarReader creates a new tar reader
func NewTarReader(r io.Reader) *TarReader {
	return &TarReader{r: r}
}

// ReadEntries reads all entries from a tar stream
func (tr *TarReader) ReadEntries(ctx context.Context) ([]*tar.Header, error) {
	var entries []*tar.Header
	
	trr := tar.NewReader(tr.r)
	for {
		hdr, err := trr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}
		
		entries = append(entries, hdr)
		
		// Skip content
		if _, err := io.Copy(io.Discard, trr); err != nil {
			return nil, fmt.Errorf("failed to skip tar content: %w", err)
		}
	}
	
	return entries, nil
}

// TarEntry represents a single tar entry with its content
type TarEntry struct {
	Header  *tar.Header
	Content []byte
}

// ReadAll reads all entries with their content
func (tr *TarReader) ReadAll(ctx context.Context) ([]TarEntry, error) {
	var entries []TarEntry
	
	trr := tar.NewReader(tr.r)
	for {
		hdr, err := trr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}
		
		content, err := io.ReadAll(trr)
		if err != nil {
			return nil, fmt.Errorf("failed to read tar content: %w", err)
		}
		
		entries = append(entries, TarEntry{
			Header:  hdr,
			Content: content,
		})
	}
	
	return entries, nil
}

// DecompressTar decompresses a gzip-compressed tar stream
func DecompressTar(r io.Reader) (io.Reader, error) {
	// Try to detect if it's gzip compressed
	br := bufio.NewReader(r)
	magic, err := br.Peek(2)
	if err != nil {
		return nil, fmt.Errorf("failed to peek: %w", err)
	}
	
	if magic[0] == 0x1f && magic[1] == 0x8b {
		// It's gzip compressed
		return gzip.NewReader(br)
	}
	
	// Not compressed
	return br, nil
}
