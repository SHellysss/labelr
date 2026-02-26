package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	logger, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer logger.Close()

	logger.Info("hello %s", "world")
	logger.Error("something %s", "broke")

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "INFO") || !strings.Contains(content, "hello world") {
		t.Errorf("log missing INFO entry, got: %s", content)
	}
	if !strings.Contains(content, "ERROR") || !strings.Contains(content, "something broke") {
		t.Errorf("log missing ERROR entry, got: %s", content)
	}
}
