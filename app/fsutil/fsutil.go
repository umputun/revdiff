// Package fsutil provides shared filesystem utilities.
package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:generate moq -out temp_file_moq_test.go -pkg fsutil -skip-ensure -fmt goimports . tempFile

// tempFile is the open temp file AtomicWriteFile fills before renaming it into place.
// *os.File satisfies it.
type tempFile interface {
	io.Writer
	io.Closer
}

// AtomicWriteFile writes data to a temp file and renames it into place,
// ensuring the write is atomic on POSIX filesystems.
func AtomicWriteFile(path string, data []byte) error {
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	return commitTemp(f, f.Name(), path, data)
}

// commitTemp writes data through w, closes it and renames tmp over path. The temp file is
// removed on every failure so a partial write never survives next to the target.
func commitTemp(w tempFile, tmp, path string, data []byte) error {
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := w.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
