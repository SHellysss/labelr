package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type Logger struct {
	file   *os.File
	logger *log.Logger
}

func New(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &Logger{
		file:   f,
		logger: log.New(f, "", log.LstdFlags),
	}, nil
}

func (l *Logger) Info(format string, args ...any) {
	l.logger.Printf("INFO  "+format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.logger.Printf("ERROR "+format, args...)
}

func (l *Logger) Debug(format string, args ...any) {
	l.logger.Printf("DEBUG "+format, args...)
}

func (l *Logger) Close() error {
	return l.file.Close()
}

func (l *Logger) Path() string {
	return l.file.Name()
}

// Rotate checks file size and rotates if needed.
func (l *Logger) Rotate(maxSize int64, keepCount int) error {
	info, err := l.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < maxSize {
		return nil
	}

	l.file.Close()
	path := l.file.Name()

	// Rotate existing files
	for i := keepCount - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", path, i)
		new := fmt.Sprintf("%s.%d", path, i+1)
		os.Rename(old, new)
	}
	os.Rename(path, path+".1")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	l.file = f
	l.logger = log.New(f, "", log.LstdFlags)
	return nil
}
