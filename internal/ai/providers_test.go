package ai

import (
	"testing"
)

func TestGetProvider(t *testing.T) {
	p, err := GetProvider("openai")
	if err != nil {
		t.Fatalf("GetProvider(openai) failed: %v", err)
	}
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("got baseURL %q", p.BaseURL)
	}
}

func TestGetProviderOllama(t *testing.T) {
	p, err := GetProvider("ollama")
	if err != nil {
		t.Fatalf("GetProvider(ollama) failed: %v", err)
	}
	if p.EnvKey != "" {
		t.Error("ollama should not require an API key")
	}
}

func TestGetProviderUnknown(t *testing.T) {
	_, err := GetProvider("nonexistent")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestListProviders(t *testing.T) {
	providers := ListProviders()
	if len(providers) < 4 {
		t.Errorf("expected at least 4 providers, got %d", len(providers))
	}
}
