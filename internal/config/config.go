package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Gmail        GmailConfig `json:"gmail"`
	AI           AIConfig    `json:"ai"`
	Labels       []Label     `json:"labels"`
	PollInterval int         `json:"pollInterval"`
}

type GmailConfig struct {
	Email string `json:"email"`
}

type AIConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseURL"`
}

type Label struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// envKeyMap maps provider names to their environment variable names.
var envKeyMap = map[string]string{
	"openai":   "OPENAI_API_KEY",
	"deepseek": "DEEPSEEK_API_KEY",
	"groq":     "GROQ_API_KEY",
}

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".labelr")
}

func DefaultPath() string {
	return filepath.Join(Dir(), "config.json")
}

func CredentialsPath() string {
	return filepath.Join(Dir(), "credentials.json")
}

func DBPath() string {
	return filepath.Join(Dir(), "labelr.db")
}

func LogPath() string {
	return filepath.Join(Dir(), "logs", "daemon.log")
}

func ModelsCachePath() string {
	return filepath.Join(Dir(), "models-cache.json")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func (c *Config) ResolveAPIKey() string {
	if envName, ok := envKeyMap[c.AI.Provider]; ok {
		if val := os.Getenv(envName); val != "" {
			return val
		}
	}
	return c.AI.APIKey
}

func DefaultLabels() []Label {
	return []Label{
		{Name: "Action Required", Description: "Emails requiring a response or action from me"},
		{Name: "Informational", Description: "FYI emails, no action needed"},
		{Name: "Newsletter", Description: "Newsletters, digests, mailing lists"},
		{Name: "Finance", Description: "Bills, receipts, banking, payments"},
		{Name: "Scheduling", Description: "Calendar invites, meeting requests"},
		{Name: "Personal", Description: "Personal emails from friends and family"},
		{Name: "Automated", Description: "Automated notifications, alerts, system emails"},
	}
}
