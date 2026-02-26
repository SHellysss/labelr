package gmail

import (
	"testing"
)

func TestExtractPlainText(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		maxLen   int
		expected string
	}{
		{"short text", "Hello world", 500, "Hello world"},
		{"truncate", "Hello world", 5, "Hello"},
		{"empty", "", 500, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.body, tt.maxLen)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractEmailData(t *testing.T) {
	headers := []MessageHeader{
		{Name: "From", Value: "alice@example.com"},
		{Name: "Subject", Value: "Test Subject"},
		{Name: "To", Value: "bob@example.com"},
	}

	data := extractEmailHeaders(headers)
	if data.From != "alice@example.com" {
		t.Errorf("got from=%q, want alice@example.com", data.From)
	}
	if data.Subject != "Test Subject" {
		t.Errorf("got subject=%q, want Test Subject", data.Subject)
	}
}
