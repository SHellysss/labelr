package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pankajbeniwal/labelr/internal/config"
)

type Provider struct {
	Name    string
	BaseURL string
	EnvKey  string // Empty string means no API key required (e.g., Ollama)
}

var providers = map[string]Provider{
	"openai": {
		Name:    "OpenAI",
		BaseURL: "https://api.openai.com/v1",
		EnvKey:  "OPENAI_API_KEY",
	},
	"ollama": {
		Name:    "Ollama (local)",
		BaseURL: "http://localhost:11434/v1",
		EnvKey:  "",
	},
	"deepseek": {
		Name:    "DeepSeek",
		BaseURL: "https://api.deepseek.com/v1",
		EnvKey:  "DEEPSEEK_API_KEY",
	},
	"groq": {
		Name:    "Groq",
		BaseURL: "https://api.groq.com/openai/v1",
		EnvKey:  "GROQ_API_KEY",
	},
}

// ProviderBaseURL returns the API base URL for a provider.
func ProviderBaseURL(provider string) string {
	p, ok := providers[provider]
	if !ok {
		return ""
	}
	return p.BaseURL
}

// EnvKeyForProvider returns the environment variable name for a provider's API key.
func EnvKeyForProvider(provider string) string {
	p, ok := providers[provider]
	if !ok {
		return ""
	}
	return p.EnvKey
}

// modelsDevKey maps our provider names to models.dev JSON keys.
var modelsDevKey = map[string]string{
	"openai":   "openai",
	"deepseek": "deepseek",
	"groq":     "groq",
}

func GetProvider(name string) (*Provider, error) {
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return &p, nil
}

func ListProviders() []Provider {
	result := make([]Provider, 0, len(providers))
	for _, p := range providers {
		result = append(result, p)
	}
	return result
}

// providerOrder is the fixed display order for provider selection.
var providerOrder = []string{"openai", "deepseek", "groq", "ollama"}

// ProviderNamesOrdered returns provider names in a fixed, deterministic order.
func ProviderNamesOrdered() []string {
	return append([]string{}, providerOrder...)
}

// modelsDevProvider represents a provider entry in the models.dev API response.
// The models field is a map keyed by model ID, not an array.
type modelsDevProvider struct {
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	StructuredOutput *bool  `json:"structured_output,omitempty"`
}

// modelsCacheEntry is what we store in the cache file.
type modelsCacheEntry struct {
	FetchedAt time.Time           `json:"fetched_at"`
	Data      map[string][]string `json:"data"` // provider key -> list of model IDs
}

const modelsCacheTTL = 7 * 24 * time.Hour

// modelsDevURL is the endpoint for fetching model metadata. Overridable in tests.
var modelsDevURL = "https://models.dev/api.json"

// FetchModelsForProvider returns model IDs that support structured output for a cloud provider.
// Uses a local cache with a 7-day TTL. Returns nil for ollama (use FetchOllamaModels instead).
func FetchModelsForProvider(providerName string) ([]string, error) {
	devKey, ok := modelsDevKey[providerName]
	if !ok {
		return nil, nil // ollama or unknown — no models.dev lookup
	}

	// Try cache first
	cachePath := config.ModelsCachePath()
	if models, ok := readModelsCache(cachePath, devKey); ok {
		return models, nil
	}

	// Fetch from models.dev
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(modelsDevURL)
	if err != nil {
		return nil, fmt.Errorf("fetching models.dev: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}

	var allProviders map[string]modelsDevProvider
	if err := json.NewDecoder(resp.Body).Decode(&allProviders); err != nil {
		return nil, fmt.Errorf("parsing models.dev response: %w", err)
	}

	// Extract structured-output models for all our providers and cache them
	cache := modelsCacheEntry{
		FetchedAt: time.Now(),
		Data:      make(map[string][]string),
	}
	for key := range modelsDevKey {
		if p, ok := allProviders[key]; ok {
			var models []string
			for _, m := range p.Models {
				// Include if structured_output is true or unknown (nil).
				// Exclude only when explicitly false.
				if m.StructuredOutput == nil || *m.StructuredOutput {
					if m.ID != "" {
						models = append(models, m.ID)
					}
				}
			}
			cache.Data[key] = models
		}
	}

	// Write cache (best-effort)
	writeModelsCache(cachePath, &cache)

	return cache.Data[devKey], nil
}

func readModelsCache(path, providerKey string) ([]string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cache modelsCacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if time.Since(cache.FetchedAt) > modelsCacheTTL {
		return nil, false
	}
	models, ok := cache.Data[providerKey]
	return models, ok && len(models) > 0
}

func writeModelsCache(path string, cache *modelsCacheEntry) {
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, data, 0600)
}

// ollamaModel represents an Ollama model from /api/tags.
type ollamaModel struct {
	Name string `json:"name"`
}

// ollamaBaseURL is the base URL for Ollama. Overridable in tests.
var ollamaBaseURL = "http://localhost:11434"

// ollamaTagsResponse represents the /api/tags response (all pulled models).
type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

// FetchOllamaModels queries Ollama /api/tags for all pulled models.
func FetchOllamaModels() ([]string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(ollamaBaseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at localhost:11434 — is it running?\n\n  To install: https://ollama.com/download\n  To start:   ollama serve")
	}
	defer resp.Body.Close()

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("parsing Ollama response: %w", err)
	}

	if len(tags.Models) == 0 {
		return nil, fmt.Errorf("no models found in Ollama\n\n  To pull a model:  ollama pull llama3\n  To run a model:   ollama run llama3")
	}

	names := make([]string, len(tags.Models))
	for i, m := range tags.Models {
		names[i] = m.Name
	}
	return names, nil
}
