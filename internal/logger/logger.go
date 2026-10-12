package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger wraps *slog.Logger and owns the file handle it opened, so callers can
// release it. The handle otherwise lives for the whole process, which matters for
// tests and short-lived tools (Windows keeps the file locked until closed).
type Logger struct {
	*slog.Logger
	file *os.File
}

// Close closes the underlying log file, if any. It is safe to call more than once.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// New sends structured operational records to the console and a project-local
// JSONL log.
func New(root string) (*Logger, error) {
	dir := filepath.Join(root, ".deploytool", "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "deploytool.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &Logger{
		Logger: slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, f), nil)),
		file:   f,
	}, nil
}
