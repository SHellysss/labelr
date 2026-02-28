package ai

import (
	"testing"

	"github.com/Pankaj3112/labelr/internal/config"
)

func TestBuildPrompt(t *testing.T) {
	labels := []config.Label{
		{Name: "Finance", Description: "Money stuff"},
		{Name: "Personal", Description: "Friends and family"},
	}

	prompt := buildPrompt("alice@example.com", "Invoice #123", "Please find attached invoice", labels)

	if prompt == "" {
		t.Fatal("prompt is empty")
	}

	// Should contain email data
	if !containsString(prompt, "alice@example.com") {
		t.Error("prompt missing sender")
	}
	if !containsString(prompt, "Invoice #123") {
		t.Error("prompt missing subject")
	}

	// Should contain labels
	if !containsString(prompt, "Finance") {
		t.Error("prompt missing Finance label")
	}
	if !containsString(prompt, "Personal") {
		t.Error("prompt missing Personal label")
	}
}

func TestBuildResponseSchema(t *testing.T) {
	labels := []config.Label{
		{Name: "Finance", Description: "Money"},
		{Name: "Personal", Description: "Friends"},
	}

	schema := buildResponseSchema(labels)
	if schema == nil {
		t.Fatal("schema is nil")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
