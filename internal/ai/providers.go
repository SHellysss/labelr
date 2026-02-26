package ai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

func ProviderNames() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// modelsDevProvider represents a provider entry in the models.dev API response.
type modelsDevProvider struct {
	Models []modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	StructuredOutput bool   `json:"structured_output"`
}

// modelsCacheEntry is what we store in the cache file.
type modelsCacheEntry struct {
	FetchedAt time.Time           `json:"fetched_at"`
	Data      map[string][]string `json:"data"` // provider key -> list of model IDs
}

const modelsCacheTTL = 7 * 24 * time.Hour

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
	resp, err := client.Get("https://models.dev/api.json")
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
				if m.StructuredOutput {
					models = append(models, m.ID)
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
	_ = os.WriteFile(path, data, 0600)
}

// OllamaModel represents a running Ollama model from /api/ps.
type OllamaModel struct {
	Name string `json:"name"`
}

type ollamaPsResponse struct {
	Models []OllamaModel `json:"models"`
}

// FetchOllamaModels queries localhost:11434/api/ps for running models.
// Returns the model list, or an error if Ollama is not reachable.
func FetchOllamaModels() ([]string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/ps")
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at localhost:11434 — is it running?\n\n  To install: https://ollama.com/download\n  To start:   ollama serve")
	}
	defer resp.Body.Close()

	var ps ollamaPsResponse
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, fmt.Errorf("parsing Ollama response: %w", err)
	}

	if len(ps.Models) == 0 {
		return nil, fmt.Errorf("no models running in Ollama\n\n  To pull a model:  ollama pull llama3\n  To run a model:   ollama run llama3")
	}

	names := make([]string, len(ps.Models))
	for i, m := range ps.Models {
		names[i] = m.Name
	}
	return names, nil
}
