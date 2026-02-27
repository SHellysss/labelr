# UX Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `labelr init` + `labelr config` with a unified `labelr setup` command, fix all UX bugs, and make `labelr uninstall` a full uninstall.

**Architecture:** Single `setup.go` replaces both `init.go` and `config_cmd.go`. First-run detection via `config.Load()` — if config exists, show reconfigure menu; if not, run full wizard. Shared helpers for AI setup, label management, and daemon restart.

**Tech Stack:** Go, cobra, charmbracelet/huh, Gmail API, SQLite

---

### Task 1: Fix provider ordering — deterministic provider list

**Files:**
- Modify: `internal/ai/providers.go:65-71`
- Modify: `internal/ai/providers_test.go`

**Step 1: Write the failing test**

Add to `internal/ai/providers_test.go`:

```go
func TestProviderNamesOrdered(t *testing.T) {
	names := ProviderNamesOrdered()
	expected := []string{"openai", "deepseek", "groq", "ollama"}
	if len(names) != len(expected) {
		t.Fatalf("got %d providers, want %d", len(names), len(expected))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("index %d: got %q, want %q", i, name, expected[i])
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/ai/ -run TestProviderNamesOrdered -v`
Expected: FAIL — `ProviderNamesOrdered` not defined

**Step 3: Write minimal implementation**

Add to `internal/ai/providers.go`:

```go
// providerOrder is the fixed display order for provider selection.
var providerOrder = []string{"openai", "deepseek", "groq", "ollama"}

// ProviderNamesOrdered returns provider names in a fixed, deterministic order.
func ProviderNamesOrdered() []string {
	return append([]string{}, providerOrder...)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/ai/ -run TestProviderNamesOrdered -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ai/providers.go internal/ai/providers_test.go
git commit -m "feat: add deterministic provider ordering"
```

---

### Task 2: Fix Ollama to use `/api/tags` instead of `/api/ps`

**Files:**
- Modify: `internal/ai/providers.go:178-211`

**Step 1: Write the failing test**

Add to `internal/ai/providers_test.go`:

```go
func TestFetchOllamaModels_UsesApiTags(t *testing.T) {
	// Mock Ollama /api/tags endpoint
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
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/ai/ -run TestFetchOllamaModels -v`
Expected: FAIL — `ollamaBaseURL` not defined

**Step 3: Write minimal implementation**

Replace the Ollama section in `internal/ai/providers.go`:

```go
// ollamaBaseURL is the base URL for Ollama. Overridable in tests.
var ollamaBaseURL = "http://localhost:11434"

// ollamaTagsResponse represents the /api/tags response (all pulled models).
type ollamaTagsResponse struct {
	Models []OllamaModel `json:"models"`
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
```

Remove the old `ollamaPsResponse` struct and the old `FetchOllamaModels` function.

**Step 4: Run test to verify it passes**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/ai/ -run TestFetchOllamaModels -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ai/providers.go internal/ai/providers_test.go
git commit -m "fix: use /api/tags for Ollama to show all pulled models"
```

---

### Task 3: Add color columns to label_mappings in DB

**Files:**
- Modify: `internal/db/db.go`
- Modify: `internal/db/db_test.go`

**Step 1: Write the failing test**

Add to `internal/db/db_test.go`:

```go
func TestSetLabelMappingWithColor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	err := store.SetLabelMappingWithColor("Finance", "Label_123", "#16a766", "#ffffff")
	if err != nil {
		t.Fatalf("SetLabelMappingWithColor failed: %v", err)
	}

	gmailID, bg, tx, err := store.GetLabelMappingWithColor("Finance")
	if err != nil {
		t.Fatalf("GetLabelMappingWithColor failed: %v", err)
	}
	if gmailID != "Label_123" {
		t.Errorf("gmailID: got %q, want Label_123", gmailID)
	}
	if bg != "#16a766" {
		t.Errorf("bg: got %q, want #16a766", bg)
	}
	if tx != "#ffffff" {
		t.Errorf("tx: got %q, want #ffffff", tx)
	}
}

func TestDeleteLabelMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.SetLabelMapping("Finance", "Label_123")
	err := store.DeleteLabelMapping("Finance")
	if err != nil {
		t.Fatalf("DeleteLabelMapping failed: %v", err)
	}

	_, err = store.GetLabelMapping("Finance")
	if err == nil {
		t.Error("expected error after deleting label mapping")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/db/ -run "TestSetLabelMappingWithColor|TestDeleteLabelMapping" -v`
Expected: FAIL — functions not defined

**Step 3: Write minimal implementation**

Add migration in `internal/db/db.go` `migrate()` method — append to the `migrations` slice:

```go
// Add color columns to label_mappings (safe to run on existing DBs)
`ALTER TABLE label_mappings ADD COLUMN bg_color TEXT NOT NULL DEFAULT ''`,
`ALTER TABLE label_mappings ADD COLUMN text_color TEXT NOT NULL DEFAULT ''`,
```

Note: SQLite ALTER TABLE ADD COLUMN will fail if column already exists. Wrap in a helper that ignores "duplicate column" errors:

```go
func (s *Store) migrateAddColumn(table, column, colDef string) {
	_, _ = s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef))
}
```

Call it after the main migrations:

```go
s.migrateAddColumn("label_mappings", "bg_color", "TEXT NOT NULL DEFAULT ''")
s.migrateAddColumn("label_mappings", "text_color", "TEXT NOT NULL DEFAULT ''")
```

Add methods:

```go
func (s *Store) SetLabelMappingWithColor(name, gmailID, bgColor, textColor string) error {
	_, err := s.db.Exec(
		`INSERT INTO label_mappings (name, gmail_id, bg_color, text_color) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET gmail_id = ?, bg_color = ?, text_color = ?`,
		name, gmailID, bgColor, textColor, gmailID, bgColor, textColor,
	)
	return err
}

func (s *Store) GetLabelMappingWithColor(name string) (gmailID, bgColor, textColor string, err error) {
	err = s.db.QueryRow(
		`SELECT gmail_id, bg_color, text_color FROM label_mappings WHERE name = ?`, name,
	).Scan(&gmailID, &bgColor, &textColor)
	return
}

func (s *Store) DeleteLabelMapping(name string) error {
	_, err := s.db.Exec(`DELETE FROM label_mappings WHERE name = ?`, name)
	return err
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./internal/db/ -v`
Expected: ALL PASS (including existing tests — backward compatible)

**Step 5: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go
git commit -m "feat: add color columns and delete method to label_mappings"
```

---

### Task 4: Add `DeleteLabel` method to Gmail client

**Files:**
- Modify: `internal/gmail/client.go`
- Modify: `internal/gmail/client_test.go`

**Step 1: Write the implementation**

Add to `internal/gmail/client.go`:

```go
// DeleteLabel deletes a Gmail label by its ID.
func (c *Client) DeleteLabel(ctx context.Context, labelID string) error {
	return c.svc.Users.Labels.Delete("me", labelID).Context(ctx).Do()
}
```

**Step 2: Verify build compiles**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/gmail/client.go
git commit -m "feat: add DeleteLabel method to Gmail client"
```

---

### Task 5: Update `labelr start` error message

**Files:**
- Modify: `internal/cli/start.go:23`

**Step 1: Update the error message**

Change line 23 from:
```go
return fmt.Errorf("no config found — run 'labelr init' first")
```
to:
```go
return fmt.Errorf("no config found — run 'labelr setup' first")
```

**Step 2: Verify build**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/cli/start.go
git commit -m "fix: update start error message to reference 'setup' instead of 'init'"
```

---

### Task 6: Create unified `setup.go` — first-run wizard

**Files:**
- Create: `internal/cli/setup.go`
- Delete: `internal/cli/init.go`
- Delete: `internal/cli/config_cmd.go`
- Modify: `cmd/labelr/main.go`

This is the largest task. The setup command has two modes: first-run (no config) and reconfigure (config exists).

**Step 1: Create `internal/cli/setup.go` with the command registration and mode detection**

```go
package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"golang.org/x/oauth2"

	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up or reconfigure labelr",
		Long:  "First-time setup wizard or reconfigure existing settings: Gmail auth, AI provider, labels.",
		RunE:  runSetup,
	}
}

func runSetup(cmd *cobra.Command, args []string) error {
	fmt.Println()

	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Detect mode: first-run vs reconfigure
	existingCfg, cfgErr := config.Load(config.DefaultPath())
	if cfgErr == nil && existingCfg.AI.Provider != "" {
		return runReconfigure(existingCfg)
	}

	return runFirstTimeSetup()
}
```

**Step 2: Implement `runFirstTimeSetup()` in the same file**

This is the full wizard flow — adapted from current `init.go` with fixes applied:

```go
func runFirstTimeSetup() error {
	ui.Bold("Welcome to labelr!")

	// --- Gmail OAuth ---
	ts, email, historyID, err := setupGmail()
	if err != nil {
		return err
	}

	client, err := gmailpkg.NewClient(context.Background(), ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}

	// --- AI Provider ---
	provider, model, apiKey, err := setupAI(nil)
	if err != nil {
		return err
	}

	// --- Labels ---
	labels, err := setupLabels(nil)
	if err != nil {
		return err
	}

	// Save config
	providerInfo, _ := ai.GetProvider(provider)
	cfg := &config.Config{
		Gmail:        config.GmailConfig{Email: email},
		AI:           config.AIConfig{Provider: provider, Model: model, APIKey: apiKey, BaseURL: providerInfo.BaseURL},
		Labels:       labels,
		PollInterval: 60,
	}
	if err := config.Save(config.DefaultPath(), cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Create labels in Gmail
	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	createLabelsInGmail(client, store, labels)

	// Store initial historyId
	store.SetState("history_id", fmt.Sprintf("%d", historyID))

	// Offer test run
	offerTestRun(client, store)

	// Auto-start daemon
	startDaemon()

	return nil
}
```

**Step 3: Implement shared helper `setupGmail()`**

```go
func setupGmail() (oauth2.TokenSource, string, uint64, error) {
	// Check if valid credentials already exist
	existingTS, loadErr := gmailpkg.TokenSource(config.CredentialsPath())
	if loadErr == nil {
		if _, tokErr := existingTS.Token(); tokErr == nil {
			client, err := gmailpkg.NewClient(context.Background(), existingTS)
			if err == nil {
				email, historyID, err := client.GetProfile(context.Background())
				if err == nil {
					ui.Success(fmt.Sprintf("Gmail already connected (%s)", email))
					return existingTS, email, historyID, nil
				}
			}
		}
	}

	ui.Header("Connect your Gmail account")
	ui.Dim("Opening browser to sign in...")
	fmt.Println()

	token, err := gmailpkg.Authenticate(config.CredentialsPath())
	if err != nil {
		return nil, "", 0, fmt.Errorf("Gmail authentication failed: %w", err)
	}
	_ = token

	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return nil, "", 0, fmt.Errorf("creating token source: %w", err)
	}

	client, err := gmailpkg.NewClient(context.Background(), ts)
	if err != nil {
		return nil, "", 0, fmt.Errorf("creating Gmail client: %w", err)
	}
	email, historyID, err := client.GetProfile(context.Background())
	if err != nil {
		return nil, "", 0, fmt.Errorf("getting profile: %w", err)
	}
	ui.Success(fmt.Sprintf("Connected as %s", email))

	return ts, email, historyID, nil
}
```

**Step 4: Implement shared helper `setupAI(existingCfg *config.Config)`**

Returns `(provider, model, apiKey string, err error)`. If `existingCfg` is nil, it's a first-time setup. If non-nil, it's a reconfigure (used by "AI provider / model" menu option).

```go
func setupAI(existingCfg *config.Config) (string, string, string, error) {
	for {
		ui.Header("Choose your AI provider")

		providerNames := ai.ProviderNamesOrdered()
		var selectedProvider string
		huh.NewSelect[string]().
			Title("Which AI provider?").
			Options(huh.NewOptions(providerNames...)...).
			Value(&selectedProvider).
			Run()

		provider, _ := ai.GetProvider(selectedProvider)

		// Model selection
		model, err := selectModel(selectedProvider)
		if err != nil {
			return "", "", "", err
		}

		// API Key
		apiKey := ""
		if provider.EnvKey != "" {
			if envVal := os.Getenv(provider.EnvKey); envVal != "" {
				ui.Info(fmt.Sprintf("Found API key in $%s", provider.EnvKey))
				apiKey = envVal
			} else if existingCfg != nil && existingCfg.AI.Provider == selectedProvider && existingCfg.AI.APIKey != "" {
				// Same provider, offer to reuse
				var reuseKey bool
				huh.NewConfirm().
					Title("Use existing API key?").
					Value(&reuseKey).
					Run()
				if reuseKey {
					apiKey = existingCfg.AI.APIKey
				} else {
					huh.NewInput().
						Title(fmt.Sprintf("Enter your %s API key:", provider.Name)).
						Value(&apiKey).
						EchoMode(huh.EchoModePassword).
						Run()
				}
			} else {
				huh.NewInput().
					Title(fmt.Sprintf("Enter your %s API key:", provider.Name)).
					Value(&apiKey).
					EchoMode(huh.EchoModePassword).
					Run()
			}
		}

		// Validate connection
		classifier := ai.NewClassifier(apiKey, provider.BaseURL, model, config.DefaultLabels())

		var validateErr error
		spinErr := spinner.New().
			Title("Verifying connection...").
			Action(func() {
				validateErr = classifier.ValidateConnection(context.Background())
			}).
			Run()
		if spinErr != nil {
			return "", "", "", spinErr
		}

		if validateErr == nil {
			ui.Success(fmt.Sprintf("Connected to %s / %s", selectedProvider, model))
			return selectedProvider, model, apiKey, nil
		}

		ui.Error(fmt.Sprintf("Could not connect to %s / %s", selectedProvider, model))
		ui.Dim("This could mean: invalid API key, model doesn't support structured output, or network issue")
		fmt.Println()

		var retry bool
		huh.NewConfirm().
			Title("Try again with different settings?").
			Value(&retry).
			Run()

		if !retry {
			return "", "", "", fmt.Errorf("setup cancelled")
		}
	}
}
```

**Step 5: Implement `setupModelOnly(cfg *config.Config)` for "Just the model" option**

```go
func setupModelOnly(cfg *config.Config) (string, error) {
	model, err := selectModel(cfg.AI.Provider)
	if err != nil {
		return "", err
	}

	// Validate with existing API key
	provider, _ := ai.GetProvider(cfg.AI.Provider)
	apiKey := cfg.ResolveAPIKey()
	classifier := ai.NewClassifier(apiKey, provider.BaseURL, model, cfg.Labels)

	var validateErr error
	spinErr := spinner.New().
		Title("Verifying connection...").
		Action(func() {
			validateErr = classifier.ValidateConnection(context.Background())
		}).
		Run()
	if spinErr != nil {
		return "", spinErr
	}

	if validateErr != nil {
		ui.Error(fmt.Sprintf("Could not connect to %s / %s", cfg.AI.Provider, model))
		ui.Dim("This could mean: model doesn't support structured output or network issue")
		return "", fmt.Errorf("validation failed")
	}

	ui.Success(fmt.Sprintf("Connected to %s / %s", cfg.AI.Provider, model))
	return model, nil
}
```

**Step 6: Implement shared helper `setupLabels(existingLabels []config.Label)`**

If `existingLabels` is nil, shows defaults. If non-nil, shows existing labels.

```go
func setupLabels(existingLabels []config.Label) ([]config.Label, error) {
	ui.Header("Configure labels")

	var sourceLabels []config.Label
	if existingLabels != nil {
		sourceLabels = existingLabels
	} else {
		sourceLabels = config.DefaultLabels()
	}

	options := make([]huh.Option[string], len(sourceLabels))
	selectedNames := make([]string, len(sourceLabels))
	for i, l := range sourceLabels {
		options[i] = huh.NewOption(fmt.Sprintf("%s — %s", l.Name, l.Description), l.Name).Selected(true)
		selectedNames[i] = l.Name
	}

	title := "Which default labels do you want?"
	if existingLabels != nil {
		title = "Keep which labels? (deselect to remove)"
	}

	huh.NewMultiSelect[string]().
		Title(title).
		Options(options...).
		Value(&selectedNames).
		Run()

	selectedSet := make(map[string]bool)
	for _, n := range selectedNames {
		selectedSet[n] = true
	}
	var labels []config.Label
	for _, l := range sourceLabels {
		if selectedSet[l.Name] {
			labels = append(labels, l)
		}
	}

	// Custom labels
	for {
		var addMore bool
		huh.NewConfirm().
			Title("Add a custom label?").
			Value(&addMore).
			Run()
		if !addMore {
			break
		}

		var name, description string
		huh.NewInput().Title("Label name:").Value(&name).Run()
		huh.NewInput().Title("Description (helps AI classify):").Value(&description).Run()

		if name != "" {
			labels = append(labels, config.Label{Name: name, Description: description})
			ui.Success(fmt.Sprintf("Added: %s", name))
		}
	}

	return labels, nil
}
```

**Step 7: Implement `createLabelsInGmail` helper**

```go
func createLabelsInGmail(client *gmailpkg.Client, store *db.Store, labels []config.Label) {
	var labelErr error
	spinErr := spinner.New().
		Title("Creating Gmail labels...").
		Action(func() {
			customIdx := 0
			for _, l := range labels {
				// Check if label already has a color stored in DB
				_, existingBg, existingTx, dbErr := store.GetLabelMappingWithColor(l.Name)
				var bg, tx string
				if dbErr == nil && existingBg != "" {
					// Preserve existing color
					bg, tx = existingBg, existingTx
				} else {
					// Assign new color
					bg, tx = gmailpkg.ColorForLabel(l.Name, customIdx)
				}

				if !gmailpkg.IsDefaultLabel(l.Name) {
					customIdx++
				}

				gmailID, err := client.CreateLabel(context.Background(), l.Name, bg, tx)
				if err != nil {
					labelErr = err
					continue
				}
				store.SetLabelMappingWithColor(l.Name, gmailID, bg, tx)
			}
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error creating labels: %v", spinErr))
		return
	}

	if labelErr != nil {
		ui.Error(fmt.Sprintf("Some labels could not be created: %v", labelErr))
	}
	for _, l := range labels {
		ui.Success(l.Name)
	}
}
```

**Step 8: Implement `offerTestRun` helper**

```go
func offerTestRun(client *gmailpkg.Client, store *db.Store) {
	var testRun bool
	huh.NewConfirm().
		Title("Label your 10 most recent emails to test?").
		Value(&testRun).
		Run()

	if !testRun {
		return
	}

	var msgs []struct{ ID, ThreadID string }
	var fetchErr error
	spinErr := spinner.New().
		Title("Fetching recent emails...").
		Action(func() {
			msgs, fetchErr = client.ListRecentMessages(context.Background(), 10)
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error: %v", spinErr))
		return
	}
	if fetchErr != nil {
		ui.Error(fmt.Sprintf("Could not fetch recent messages: %v", fetchErr))
		return
	}

	for _, m := range msgs {
		store.InsertMessage(m.ID, m.ThreadID)
	}
	ui.Success(fmt.Sprintf("Queued %d emails for labeling", len(msgs)))
}
```

**Step 9: Implement `startDaemon` and `restartDaemon` helpers**

```go
func startDaemon() {
	mgr := service.Detect()
	if mgr == nil {
		ui.Dim("Could not detect service manager — run 'labelr daemon' manually")
		return
	}

	if running, _ := mgr.IsRunning(); running {
		restartDaemon(mgr)
		return
	}

	spinErr := spinner.New().
		Title("Starting labelr...").
		Action(func() {
			mgr.Install(findBinary())
			mgr.Start()
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error starting daemon: %v", spinErr))
		return
	}
	ui.Success("labelr is running in the background")
	ui.Dim("Use 'labelr status' to check on it")
	fmt.Println()
}

func restartDaemon(mgr service.Manager) {
	spinErr := spinner.New().
		Title("Restarting daemon with new config...").
		Action(func() {
			mgr.Stop()
			mgr.Install(findBinary())
			mgr.Start()
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error restarting daemon: %v", spinErr))
		return
	}
	ui.Success("Daemon restarted")
}

func restartDaemonIfRunning() {
	mgr := service.Detect()
	if mgr == nil {
		return
	}
	if running, _ := mgr.IsRunning(); running {
		restartDaemon(mgr)
	}
}

// findBinary returns the path to the labelr binary.
func findBinary() string {
	if path, err := os.Executable(); err == nil {
		return path
	}
	return "labelr"
}
```

**Step 10: Implement `runReconfigure()` — the re-run menu**

```go
func runReconfigure(cfg *config.Config) error {
	for {
		// Show current config
		ui.Bold("Current configuration")
		fmt.Println("  ──────────────────────")
		ui.KeyValue("Gmail", cfg.Gmail.Email)
		ui.KeyValue("Provider", cfg.AI.Provider)
		ui.KeyValue("Model", cfg.AI.Model)

		labelNames := make([]string, len(cfg.Labels))
		for i, l := range cfg.Labels {
			labelNames[i] = l.Name
		}
		ui.KeyValue("Labels", fmt.Sprintf("%d labels", len(cfg.Labels)))
		ui.KeyValue("Poll", fmt.Sprintf("%ds", cfg.PollInterval))
		fmt.Println()

		var editChoice string
		huh.NewSelect[string]().
			Title("What would you like to change?").
			Options(
				huh.NewOption("Nothing, exit", "none"),
				huh.NewOption("Gmail account", "gmail"),
				huh.NewOption("AI provider / model", "ai"),
				huh.NewOption("Just the model", "model"),
				huh.NewOption("Labels", "labels"),
				huh.NewOption("Poll interval", "poll"),
			).
			Value(&editChoice).
			Run()

		switch editChoice {
		case "none":
			return nil

		case "gmail":
			ui.Dim("Opening browser to sign in...")
			fmt.Println()

			_, err := gmailpkg.Authenticate(config.CredentialsPath())
			if err != nil {
				return fmt.Errorf("Gmail authentication failed: %w", err)
			}

			ts, err := gmailpkg.TokenSource(config.CredentialsPath())
			if err != nil {
				return fmt.Errorf("creating token source: %w", err)
			}
			client, err := gmailpkg.NewClient(context.Background(), ts)
			if err != nil {
				return fmt.Errorf("creating Gmail client: %w", err)
			}
			email, _, err := client.GetProfile(context.Background())
			if err != nil {
				return fmt.Errorf("getting profile: %w", err)
			}
			cfg.Gmail.Email = email
			ui.Success(fmt.Sprintf("Connected as %s", email))

		case "ai":
			provider, model, apiKey, err := setupAI(cfg)
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			providerInfo, _ := ai.GetProvider(provider)
			cfg.AI.Provider = provider
			cfg.AI.Model = model
			cfg.AI.APIKey = apiKey
			cfg.AI.BaseURL = providerInfo.BaseURL

		case "model":
			model, err := setupModelOnly(cfg)
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			cfg.AI.Model = model

		case "labels":
			newLabels, err := setupLabels(cfg.Labels)
			if err != nil {
				return err
			}

			// Find removed labels and delete from Gmail
			removedLabels := findRemovedLabels(cfg.Labels, newLabels)
			if len(removedLabels) > 0 {
				removeLabelsFromGmail(removedLabels)
			}

			// Find new labels and create in Gmail
			addedLabels := findAddedLabels(cfg.Labels, newLabels)
			if len(addedLabels) > 0 {
				ts, tsErr := gmailpkg.TokenSource(config.CredentialsPath())
				if tsErr == nil {
					client, clientErr := gmailpkg.NewClient(context.Background(), ts)
					if clientErr == nil {
						store, dbErr := db.Open(config.DBPath())
						if dbErr == nil {
							defer store.Close()
							createLabelsInGmail(client, store, addedLabels)
						}
					}
				}
			}

			cfg.Labels = newLabels

		case "poll":
			interval, err := promptPollInterval()
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			cfg.PollInterval = interval
		}

		// Save after each change
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		ui.Success("Config saved")

		// Restart daemon if running
		restartDaemonIfRunning()

		fmt.Println()
	}
}
```

**Step 11: Implement label diff helpers and poll interval prompt**

```go
func findRemovedLabels(old, new []config.Label) []config.Label {
	newSet := make(map[string]bool)
	for _, l := range new {
		newSet[l.Name] = true
	}
	var removed []config.Label
	for _, l := range old {
		if !newSet[l.Name] {
			removed = append(removed, l)
		}
	}
	return removed
}

func findAddedLabels(old, new []config.Label) []config.Label {
	oldSet := make(map[string]bool)
	for _, l := range old {
		oldSet[l.Name] = true
	}
	var added []config.Label
	for _, l := range new {
		if !oldSet[l.Name] {
			added = append(added, l)
		}
	}
	return added
}

func removeLabelsFromGmail(labels []config.Label) {
	ts, tsErr := gmailpkg.TokenSource(config.CredentialsPath())
	if tsErr != nil {
		return
	}
	client, clientErr := gmailpkg.NewClient(context.Background(), ts)
	if clientErr != nil {
		return
	}
	store, dbErr := db.Open(config.DBPath())
	if dbErr != nil {
		return
	}
	defer store.Close()

	for _, l := range labels {
		gmailID, err := store.GetLabelMapping(l.Name)
		if err != nil {
			continue
		}
		if err := client.DeleteLabel(context.Background(), gmailID); err != nil {
			ui.Error(fmt.Sprintf("Could not delete label %q from Gmail: %v", l.Name, err))
			continue
		}
		store.DeleteLabelMapping(l.Name)
		ui.Info(fmt.Sprintf("Removed from Gmail: %s", l.Name))
	}
}

func promptPollInterval() (int, error) {
	for {
		var intervalStr string
		huh.NewInput().
			Title("Poll interval (seconds):").
			Value(&intervalStr).
			Run()

		interval, err := strconv.Atoi(intervalStr)
		if err != nil || interval <= 0 {
			ui.Error("Please enter a positive number")
			continue
		}
		return interval, nil
	}
}
```

**Step 12: Verify build**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: Success

**Step 13: Update `cmd/labelr/main.go`**

Replace `cli.NewInitCmd()` and `cli.NewConfigCmd()` with `cli.NewSetupCmd()`:

```go
rootCmd.AddCommand(
	cli.NewSetupCmd(),
	cli.NewDaemonCmd(),
	cli.NewStartCmd(),
	cli.NewStopCmd(),
	cli.NewStatusCmd(),
	cli.NewLogsCmd(),
	cli.NewSyncCmd(),
	cli.NewUninstallCmd(),
)
```

**Step 14: Delete old files**

```bash
rm internal/cli/init.go internal/cli/config_cmd.go
```

**Step 15: Verify build and existing tests**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./... && go test ./...`
Expected: Build success, all tests pass

**Step 16: Commit**

```bash
git add -A
git commit -m "feat: replace init + config with unified setup command"
```

---

### Task 7: Update `labelr uninstall` for full uninstall

**Files:**
- Modify: `internal/cli/uninstall.go`

**Step 1: Rewrite uninstall.go**

```go
package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Fully uninstall labelr",
		RunE:  runUninstall,
	}
}

func runUninstall(cmd *cobra.Command, args []string) error {
	fmt.Println()

	// Stop and remove service
	mgr := service.Detect()
	if mgr != nil {
		ui.Info("Stopping daemon...")
		mgr.Stop()
		if err := mgr.Uninstall(); err != nil {
			ui.Error(fmt.Sprintf("Could not remove service: %v", err))
		} else {
			ui.Success("Background service removed")
		}
	}

	// Ask about data
	var keepData bool
	huh.NewConfirm().
		Title("Keep your data (~/.labelr/)?").
		Value(&keepData).
		Run()

	if keepData {
		ui.Info(fmt.Sprintf("Data kept at %s", config.Dir()))
	} else {
		if err := os.RemoveAll(config.Dir()); err != nil {
			return fmt.Errorf("removing data: %w", err)
		}
		ui.Success("All data deleted")
	}

	// Remove binary
	binaryPath, err := os.Executable()
	if err != nil {
		ui.Error("Could not determine binary path — remove manually")
	} else {
		if err := os.Remove(binaryPath); err != nil {
			ui.Error(fmt.Sprintf("Could not remove binary at %s: %v", binaryPath, err))
			ui.Dim("You may need to remove it manually or run with sudo")
		} else {
			ui.Success("Binary removed")
		}
	}

	fmt.Println()
	ui.Bold("labelr has been uninstalled.")
	fmt.Println()
	return nil
}
```

**Step 2: Verify build**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/cli/uninstall.go
git commit -m "feat: make uninstall fully remove binary and optionally data"
```

---

### Task 8: Check `service.Manager` interface has required methods

**Files:**
- Read: `internal/service/` files

**Step 1: Verify the `service.Manager` interface**

Check that the `service.Manager` interface referenced in setup.go has `Install(binary string)`, `Start()`, `Stop()`, `IsRunning()`, `Uninstall()` methods. Read the service files to confirm.

Run: `cd /Users/pankajbeniwal/Code/labelr && grep -n "type Manager" internal/service/*.go`

If the interface matches what setup.go calls, no changes needed. If not, adjust setup.go to match the actual interface signatures.

**Step 2: Run full test suite**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./...`
Expected: ALL PASS

**Step 3: Commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: align setup with service.Manager interface"
```

---

### Task 9: Final verification — build, test, manual smoke test

**Step 1: Run full build**

Run: `cd /Users/pankajbeniwal/Code/labelr && go build -o labelr ./cmd/labelr`
Expected: Binary built successfully

**Step 2: Run full test suite**

Run: `cd /Users/pankajbeniwal/Code/labelr && go test ./... -v`
Expected: ALL PASS

**Step 3: Verify CLI help output**

Run: `./labelr --help`
Expected: Shows `setup` command, no `init` or `config`

Run: `./labelr setup --help`
Expected: Shows setup description

**Step 4: Commit any final fixes**

```bash
git add -A
git commit -m "chore: final verification and cleanup"
```
