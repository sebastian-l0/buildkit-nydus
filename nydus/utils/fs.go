package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EnsureDir ensures a directory exists
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// TempDir creates a temporary directory for conversion
func TempDir(prefix string) (string, error) {
	return os.MkdirTemp("", fmt.Sprintf("nydus-%s-*", prefix))
}

// CleanDir removes a directory and its contents
func CleanDir(path string) error {
	return os.RemoveAll(path)
}

// WriteFileAtomically writes a file atomically
func WriteFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Write to temp file first
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	
	// Rename to final path
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}
	
	return nil
}

// CopyFile copies a file
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer sourceFile.Close()
	
	return WriteFileFromReader(dst, sourceFile)
}

// WriteFileFromReader writes a file from a reader
func WriteFileFromReader(path string, r io.Reader) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	tempPath := path + ".tmp"
	destFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	
	if _, err := io.Copy(destFile, r); err != nil {
		destFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	if err := destFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close file: %w", err)
	}
	
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}
	
	return nil
}
