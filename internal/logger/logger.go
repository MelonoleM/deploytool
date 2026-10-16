package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	// DefaultMaxBytes is the size at which a log file rolls over.
	DefaultMaxBytes = 10 * 1024 * 1024 // 10 MiB
	// DefaultKeep is how many rotated backup files are retained per log stream.
	DefaultKeep = 5
)

// Logger wraps *slog.Logger and owns the rolling file it writes to, so callers can
// release it. The handle otherwise lives for the whole process, which matters for
// tests and short-lived tools (Windows keeps the file locked until closed).
type Logger struct {
	*slog.Logger
	w *rollingWriter
}

// Close closes the underlying log file, if any. It is safe to call more than once.
func (l *Logger) Close() error {
	if l == nil || l.w == nil {
		return nil
	}
	return l.w.Close()
}

// New sends structured operational records to the console and a size-rotated,
// project-local JSONL log.
func New(root string) (*Logger, error) {
	path := filepath.Join(root, ".deploytool", "logs", "deploytool.jsonl")
	w, err := newRollingWriter(path, DefaultMaxBytes, DefaultKeep)
	if err != nil {
		return nil, err
	}
	return &Logger{
		Logger: slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, w), nil)),
		w:      w,
	}, nil
}

// RotateLogFile renames path to path.1 (shifting older backups) when it has grown
// to at least maxBytes, so a caller can open a fresh file afterwards. It returns
// nil when the file does not exist or has not yet reached maxBytes. keep is the
// number of rotated backups to retain.
func RotateLogFile(path string, maxBytes int64, keep int) error {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Size() < maxBytes {
		return nil
	}
	return rotate(path, keep)
}

// rollingWriter appends to a file, rolling it over before a write that would
// exceed maxBytes. Rotated files are named path.1, path.2, ... with path.1 being
// the most recent backup.
type rollingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
	file     *os.File
	size     int64
}

func newRollingWriter(path string, maxBytes int64, keep int) (*rollingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	w := &rollingWriter{path: path, maxBytes: maxBytes, keep: keep}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rollingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = fi.Size()
	return nil
}

func (w *rollingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.maxBytes > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.roll(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rollingWriter) roll() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	if err := rotate(w.path, w.keep); err != nil && !os.IsNotExist(err) {
		return err
	}
	return w.open()
}

func (w *rollingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// rotate renames path to path.1, path.1 to path.2, and so on, retaining at most
// keep backups. The oldest backup (path.keep) is discarded.
func rotate(path string, keep int) error {
	_ = os.Remove(fmt.Sprintf("%s.%d", path, keep))
	for i := keep - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(old); err == nil {
			if err := os.Rename(old, fmt.Sprintf("%s.%d", path, i+1)); err != nil {
				return err
			}
		}
	}
	return os.Rename(path, fmt.Sprintf("%s.1", path))
}
