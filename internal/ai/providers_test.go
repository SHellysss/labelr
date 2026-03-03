package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pankaj3112/labelr/internal/config"
)

// boolPtr is a helper to create *bool values for test data.
func boolPtr(v bool) *bool { return &v }

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

func TestProviderNamesOrdered(t *testing.T) {
	names := ProviderNamesOrdered()
	expected := []string{"openai", "deepseek", "groq", "cerebras", "ollama", "custom"}
	if len(names) != len(expected) {
		t.Fatalf("got %d providers, want %d", len(names), len(expected))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("index %d: got %q, want %q", i, name, expected[i])
		}
	}
}

func TestListProviders(t *testing.T) {
	providers := ListProviders()
	if len(providers) < 5 {
		t.Errorf("expected at least 5 providers, got %d", len(providers))
	}
}

// fakeModelsDevResponse builds a realistic models.dev API JSON response
// where models is a map keyed by model ID (matching the real API structure).
// It includes three cases for structured_output: true, false, and missing (nil).
func fakeModelsDevResponse(t *testing.T) []byte {
	t.Helper()
	resp := map[string]modelsDevProvider{
		"openai": {
			Models: map[string]modelsDevModel{
				"gpt-4o": {
					ID:               "gpt-4o",
					Name:             "GPT-4o",
					StructuredOutput: boolPtr(true),
				},
				"gpt-4o-mini": {
					ID:               "gpt-4o-mini",
					Name:             "GPT-4o Mini",
					StructuredOutput: boolPtr(true),
				},
				"o1-preview": {
					ID:               "o1-preview",
					Name:             "o1 Preview",
					StructuredOutput: boolPtr(false),
				},
			},
		},
		"deepseek": {
			Models: map[string]modelsDevModel{
				"deepseek-chat": {
					ID:               "deepseek-chat",
					Name:             "DeepSeek Chat",
					StructuredOutput: nil, // missing field — should be included
				},
				"deepseek-reasoner": {
					ID:               "deepseek-reasoner",
					Name:             "DeepSeek Reasoner",
					StructuredOutput: nil, // missing field — should be included
				},
			},
		},
		"groq": {
			Models: map[string]modelsDevModel{
				"llama-3.3-70b-versatile": {
					ID:               "llama-3.3-70b-versatile",
					Name:             "Llama 3.3 70B",
					StructuredOutput: boolPtr(true),
				},
				"gemma2-9b-it": {
					ID:               "gemma2-9b-it",
					Name:             "Gemma2 9B",
					StructuredOutput: boolPtr(false),
				},
				"whisper-large-v3": {
					ID:               "whisper-large-v3",
					Name:             "Whisper Large V3",
					StructuredOutput: nil, // missing — should be included
				},
			},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshaling fake response: %v", err)
	}
	return data
}

// setupFetchTest configures a test server and temp cache dir, restoring originals on cleanup.
func setupFetchTest(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	origURL := modelsDevURL
	modelsDevURL = srv.URL
	t.Cleanup(func() { modelsDevURL = origURL })

	tmpDir := t.TempDir()
	return tmpDir
}

// clearRealCache backs up and removes the real cache file so tests hit the mock server.
func clearRealCache(t *testing.T) {
	t.Helper()
	realCachePath := config.ModelsCachePath()
	origCache, hadCache := backupFile(realCachePath)
	t.Cleanup(func() { restoreFile(realCachePath, origCache, hadCache) })
	os.Remove(realCachePath)
}

func TestParseModelsDevResponse_MapFormat(t *testing.T) {
	raw := fakeModelsDevResponse(t)

	var allProviders map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &allProviders); err != nil {
		t.Fatalf("failed to unmarshal models.dev response: %v", err)
	}

	openai, ok := allProviders["openai"]
	if !ok {
		t.Fatal("expected 'openai' key in response")
	}
	if len(openai.Models) != 3 {
		t.Errorf("expected 3 openai models, got %d", len(openai.Models))
	}

	gpt4o, ok := openai.Models["gpt-4o"]
	if !ok {
		t.Fatal("expected 'gpt-4o' in openai models")
	}
	if gpt4o.StructuredOutput == nil || !*gpt4o.StructuredOutput {
		t.Error("gpt-4o should have structured_output=true")
	}
	if gpt4o.Name != "GPT-4o" {
		t.Errorf("expected name 'GPT-4o', got %q", gpt4o.Name)
	}
}

func TestParseModelsDevResponse_NilStructuredOutput(t *testing.T) {
	raw := fakeModelsDevResponse(t)

	var allProviders map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &allProviders); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	ds := allProviders["deepseek"]
	chat := ds.Models["deepseek-chat"]
	if chat.StructuredOutput != nil {
		t.Errorf("expected nil structured_output for deepseek-chat, got %v", *chat.StructuredOutput)
	}
}

func TestParseModelsDevResponse_MissingFieldFromJSON(t *testing.T) {
	// Simulate real API where structured_output is entirely absent from the JSON.
	raw := []byte(`{
		"deepseek": {
			"models": {
				"deepseek-chat": {
					"id": "deepseek-chat",
					"name": "DeepSeek Chat"
				}
			}
		}
	}`)

	var allProviders map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &allProviders); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	m := allProviders["deepseek"].Models["deepseek-chat"]
	if m.StructuredOutput != nil {
		t.Error("expected nil when structured_output is missing from JSON")
	}
}

func TestParseModelsDevResponse_OldArrayFormat_Fails(t *testing.T) {
	raw := []byte(`{
		"openai": {
			"models": [
				{"id": "gpt-4o", "name": "GPT-4o", "structured_output": true}
			]
		}
	}`)

	var allProviders map[string]modelsDevProvider
	err := json.Unmarshal(raw, &allProviders)
	if err == nil {
		t.Error("expected error when parsing array-format models into map type")
	}
}

func TestFetchModelsForProvider_OpenAI(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fakeModelsDevResponse(t))
	})
	clearRealCache(t)

	models, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("FetchModelsForProvider(openai) failed: %v", err)
	}

	// gpt-4o (true) and gpt-4o-mini (true) included, o1-preview (false) excluded
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(models), models)
	}

	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}
	if !modelSet["gpt-4o"] {
		t.Error("expected gpt-4o in results")
	}
	if !modelSet["gpt-4o-mini"] {
		t.Error("expected gpt-4o-mini in results")
	}
	if modelSet["o1-preview"] {
		t.Error("o1-preview should not be in results (structured_output=false)")
	}
}

func TestFetchModelsForProvider_DeepSeek_NilIncluded(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fakeModelsDevResponse(t))
	})
	clearRealCache(t)

	models, err := FetchModelsForProvider("deepseek")
	if err != nil {
		t.Fatalf("FetchModelsForProvider(deepseek) failed: %v", err)
	}

	// Both models have nil structured_output — both should be included
	if len(models) != 2 {
		t.Fatalf("expected 2 models (nil means include), got %d: %v", len(models), models)
	}

	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}
	if !modelSet["deepseek-chat"] {
		t.Error("expected deepseek-chat (nil structured_output should be included)")
	}
	if !modelSet["deepseek-reasoner"] {
		t.Error("expected deepseek-reasoner (nil structured_output should be included)")
	}
}

func TestFetchModelsForProvider_Groq_MixedNilTrueFalse(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fakeModelsDevResponse(t))
	})
	clearRealCache(t)

	models, err := FetchModelsForProvider("groq")
	if err != nil {
		t.Fatalf("FetchModelsForProvider(groq) failed: %v", err)
	}

	// llama (true) + whisper (nil) included, gemma (false) excluded
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(models), models)
	}

	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}
	if !modelSet["llama-3.3-70b-versatile"] {
		t.Error("expected llama-3.3-70b-versatile (structured_output=true)")
	}
	if !modelSet["whisper-large-v3"] {
		t.Error("expected whisper-large-v3 (nil structured_output should be included)")
	}
	if modelSet["gemma2-9b-it"] {
		t.Error("gemma2-9b-it should be excluded (structured_output=false)")
	}
}

func TestFetchModelsForProvider_Ollama_ReturnsNil(t *testing.T) {
	models, err := FetchModelsForProvider("ollama")
	if err != nil {
		t.Fatalf("unexpected error for ollama: %v", err)
	}
	if models != nil {
		t.Errorf("expected nil for ollama, got %v", models)
	}
}

func TestFetchModelsForProvider_UnknownProvider(t *testing.T) {
	models, err := FetchModelsForProvider("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for unknown provider: %v", err)
	}
	if models != nil {
		t.Errorf("expected nil for unknown provider, got %v", models)
	}
}

func TestFetchModelsForProvider_ServerError(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	clearRealCache(t)

	_, err := FetchModelsForProvider("openai")
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestFetchModelsForProvider_InvalidJSON(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	})
	clearRealCache(t)

	_, err := FetchModelsForProvider("openai")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchModelsForProvider_UsesCache(t *testing.T) {
	callCount := 0
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write(fakeModelsDevResponse(t))
	})
	clearRealCache(t)

	// First call should hit the server
	_, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second call should use cache
	models, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected cache hit (1 server call), got %d", callCount)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 cached models, got %d", len(models))
	}
}

func TestReadModelsCache_ExpiredTTL(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	cache := modelsCacheEntry{
		FetchedAt: time.Now().Add(-8 * 24 * time.Hour), // 8 days ago, beyond 7-day TTL
		Data: map[string][]string{
			"openai": {"gpt-4o"},
		},
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(cachePath, data, 0600)

	models, ok := readModelsCache(cachePath, "openai")
	if ok {
		t.Errorf("expected cache miss for expired TTL, got models: %v", models)
	}
}

func TestReadModelsCache_ValidTTL(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	cache := modelsCacheEntry{
		FetchedAt: time.Now().Add(-1 * 24 * time.Hour), // 1 day ago
		Data: map[string][]string{
			"openai": {"gpt-4o", "gpt-4o-mini"},
		},
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(cachePath, data, 0600)

	models, ok := readModelsCache(cachePath, "openai")
	if !ok {
		t.Fatal("expected cache hit for valid TTL")
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
}

func TestReadModelsCache_MissingProvider(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	cache := modelsCacheEntry{
		FetchedAt: time.Now(),
		Data: map[string][]string{
			"openai": {"gpt-4o"},
		},
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(cachePath, data, 0600)

	_, ok := readModelsCache(cachePath, "groq")
	if ok {
		t.Error("expected cache miss for missing provider key")
	}
}

func TestReadModelsCache_EmptyModels(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	cache := modelsCacheEntry{
		FetchedAt: time.Now(),
		Data: map[string][]string{
			"openai": {},
		},
	}
	data, _ := json.Marshal(cache)
	os.WriteFile(cachePath, data, 0600)

	_, ok := readModelsCache(cachePath, "openai")
	if ok {
		t.Error("expected cache miss for empty model list")
	}
}

func TestReadModelsCache_MissingFile(t *testing.T) {
	_, ok := readModelsCache("/nonexistent/path/cache.json", "openai")
	if ok {
		t.Error("expected cache miss for missing file")
	}
}

func TestReadModelsCache_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")
	os.WriteFile(cachePath, []byte(`not json`), 0600)

	_, ok := readModelsCache(cachePath, "openai")
	if ok {
		t.Error("expected cache miss for corrupted file")
	}
}

func TestFetchModelsForProvider_EmptyModelsMap(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]modelsDevProvider{
			"openai": {Models: map[string]modelsDevModel{}},
		}
		json.NewEncoder(w).Encode(resp)
	})
	clearRealCache(t)

	models, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models for empty map, got %d", len(models))
	}
}

func TestFetchModelsForProvider_AllNilStructuredOutput(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]modelsDevProvider{
			"openai": {
				Models: map[string]modelsDevModel{
					"model-a": {ID: "model-a", Name: "Model A"},
					"model-b": {ID: "model-b", Name: "Model B"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	clearRealCache(t)

	models, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All nil → all included
	if len(models) != 2 {
		t.Errorf("expected 2 models (all nil should be included), got %d: %v", len(models), models)
	}
}

func TestFetchModelsForProvider_AllExplicitlyFalse(t *testing.T) {
	setupFetchTest(t, func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]modelsDevProvider{
			"openai": {
				Models: map[string]modelsDevModel{
					"model-a": {ID: "model-a", Name: "Model A", StructuredOutput: boolPtr(false)},
					"model-b": {ID: "model-b", Name: "Model B", StructuredOutput: boolPtr(false)},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	clearRealCache(t)

	models, err := FetchModelsForProvider("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All explicitly false → none included
	if len(models) != 0 {
		t.Errorf("expected 0 models (all false), got %d: %v", len(models), models)
	}
}

func TestFetchOllamaModels_UsesApiTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected /api/tags, got %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{
				{"name": "llama3:latest", "model": "llama3:latest"},
				{"name": "mistral:latest", "model": "mistral:latest"},
			},
		})
	}))
	defer srv.Close()

	origURL := ollamaBaseURL
	ollamaBaseURL = srv.URL
	defer func() { ollamaBaseURL = origURL }()

	models, err := FetchOllamaModels()
	if err != nil {
		t.Fatalf("FetchOllamaModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(models), models)
	}
}

func TestFetchOllamaModels_NoModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]interface{}{},
		})
	}))
	defer srv.Close()

	origURL := ollamaBaseURL
	ollamaBaseURL = srv.URL
	defer func() { ollamaBaseURL = origURL }()

	_, err := FetchOllamaModels()
	if err == nil {
		t.Error("expected error for no models")
	}
}

// backupFile reads a file, returning its contents for later restoration.
func backupFile(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// restoreFile restores a previously backed-up file, or removes it if it didn't exist.
func restoreFile(path string, data []byte, existed bool) {
	if existed {
		os.WriteFile(path, data, 0600)
	} else {
		os.Remove(path)
	}
}
