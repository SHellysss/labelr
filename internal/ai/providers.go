package ai

import "fmt"

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
