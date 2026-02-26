package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		Gmail: GmailConfig{Email: "test@gmail.com"},
		AI: AIConfig{
			Provider: "openai",
			Model:    "gpt-4o-mini",
			APIKey:   "sk-test",
			BaseURL:  "https://api.openai.com/v1",
		},
		Labels: []Label{
			{Name: "Finance", Description: "Money stuff"},
		},
		PollInterval: 60,
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Gmail.Email != cfg.Gmail.Email {
		t.Errorf("got email %q, want %q", loaded.Gmail.Email, cfg.Gmail.Email)
	}
	if loaded.AI.Provider != cfg.AI.Provider {
		t.Errorf("got provider %q, want %q", loaded.AI.Provider, cfg.AI.Provider)
	}
	if len(loaded.Labels) != 1 {
		t.Errorf("got %d labels, want 1", len(loaded.Labels))
	}
	if loaded.PollInterval != 60 {
		t.Errorf("got poll interval %d, want 60", loaded.PollInterval)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestAPIKeyEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		AI: AIConfig{
			Provider: "openai",
			APIKey:   "sk-from-config",
		},
	}
	Save(path, cfg)

	os.Setenv("OPENAI_API_KEY", "sk-from-env")
	defer os.Unsetenv("OPENAI_API_KEY")

	loaded, _ := Load(path)
	key := loaded.ResolveAPIKey()
	if key != "sk-from-env" {
		t.Errorf("got key %q, want sk-from-env", key)
	}
}

func TestDefaultLabels(t *testing.T) {
	labels := DefaultLabels()
	if len(labels) != 7 {
		t.Errorf("got %d default labels, want 7", len(labels))
	}
}

func TestConfigDir(t *testing.T) {
	dir := Dir()
	if dir == "" {
		t.Error("Dir() returned empty string")
	}
}
