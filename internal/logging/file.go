package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileName is the runtime log file kept under the data directory.
const FileName = "miyabi.log"

const maxLogBytes = 5 << 20

// Path returns the runtime log path for a data directory.
func Path(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

// Writer appends logs to a size-bounded file, rotating one backup when needed.
type Writer struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func Open(dataDir string) (*Writer, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	path := Path(dataDir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	return &Writer{path: path, file: file, size: info.Size()}, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > maxLogBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *Writer) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	backup := w.path + ".1"
	_ = os.Remove(backup)
	if err := os.Rename(w.path, backup); err != nil {
		// Keep the writer usable: reopen the original file and append to it.
		if file, reopenErr := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); reopenErr == nil {
			w.file = file
			if info, statErr := file.Stat(); statErr == nil {
				w.size = info.Size()
			}
		}
		return fmt.Errorf("rotate log file: %w", err)
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("reopen log file: %w", err)
	}
	w.file = file
	w.size = 0
	return nil
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
