# labelr Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a local-first CLI tool that polls Gmail, classifies emails with AI, and applies labels automatically.

**Architecture:** Single Go binary with hidden `daemon` subcommand. Two decoupled loops (poller + worker) communicate via SQLite queue. OpenAI-compatible SDK for AI, cobra+bubbletea for CLI.

**Tech Stack:** Go, cobra, bubbletea, OpenAI Go SDK v3, Google Gmail API, SQLite (modernc.org/sqlite), Models.dev API

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/labelr/main.go`
- Create: `Makefile`

**Step 1: Initialize Go module**

Run: `go mod init github.com/pankajbeniwal/labelr`

**Step 2: Create main.go with cobra root command**

```go
// cmd/labelr/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "labelr",
		Short:   "AI-powered Gmail labeler",
		Long:    "labelr automatically classifies and labels your Gmail emails using AI.",
		Version: version,
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

**Step 3: Create Makefile**

```makefile
.PHONY: build run test clean

build:
	go build -o bin/labelr ./cmd/labelr

run:
	go run ./cmd/labelr

test:
	go test ./... -v

clean:
	rm -rf bin/
```

**Step 4: Install cobra dependency and verify build**

Run: `go get github.com/spf13/cobra && make build`
Expected: Binary at `bin/labelr`

**Step 5: Verify it runs**

Run: `./bin/labelr --version`
Expected: `labelr version dev`

**Step 6: Commit**

```bash
git add go.mod go.sum cmd/ Makefile
git commit -m "feat: scaffold project with cobra root command"
```

---

### Task 2: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
// internal/config/config_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — types and functions not defined

**Step 3: Write implementation**

```go
// internal/config/config.go
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
```

**Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with load, save, defaults, env override"
```

---

### Task 3: Database Package

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/db_test.go`

**Step 1: Write the failing test**

```go
// internal/db/db_test.go
package db

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()
}

func TestInsertAndGetPendingMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	err := store.InsertMessage("msg1", "thread1")
	if err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	// Duplicate insert should not error
	err = store.InsertMessage("msg1", "thread1")
	if err != nil {
		t.Fatalf("Duplicate InsertMessage failed: %v", err)
	}

	msgs, err := store.PendingMessages(10)
	if err != nil {
		t.Fatalf("PendingMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d pending, want 1", len(msgs))
	}
	if msgs[0].ID != "msg1" {
		t.Errorf("got ID %q, want msg1", msgs[0].ID)
	}
}

func TestMarkProcessing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.InsertMessage("msg1", "thread1")
	store.MarkProcessing("msg1")

	msgs, _ := store.PendingMessages(10)
	if len(msgs) != 0 {
		t.Error("expected no pending messages after marking processing")
	}
}

func TestMarkLabeled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.InsertMessage("msg1", "thread1")
	store.MarkProcessing("msg1")
	store.MarkLabeled("msg1", "Finance")

	stats, _ := store.Stats()
	if stats.Labeled != 1 {
		t.Errorf("got labeled=%d, want 1", stats.Labeled)
	}
}

func TestMarkFailed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.InsertMessage("msg1", "thread1")
	store.MarkFailed("msg1")

	msg, _ := store.GetMessage("msg1")
	if msg.Attempts != 1 {
		t.Errorf("got attempts=%d, want 1", msg.Attempts)
	}
	if msg.Status != "pending" {
		t.Errorf("got status=%q, want pending (under max retries)", msg.Status)
	}

	// Fail 2 more times to exceed max retries
	store.MarkFailed("msg1")
	store.MarkFailed("msg1")

	msg, _ = store.GetMessage("msg1")
	if msg.Status != "failed" {
		t.Errorf("got status=%q, want failed after 3 attempts", msg.Status)
	}
}

func TestResetProcessing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.InsertMessage("msg1", "thread1")
	store.MarkProcessing("msg1")
	store.ResetProcessing()

	msgs, _ := store.PendingMessages(10)
	if len(msgs) != 1 {
		t.Error("expected processing message to be reset to pending")
	}
}

func TestStateGetSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.SetState("history_id", "12345")
	val, err := store.GetState("history_id")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if val != "12345" {
		t.Errorf("got %q, want 12345", val)
	}

	// Overwrite
	store.SetState("history_id", "67890")
	val, _ = store.GetState("history_id")
	if val != "67890" {
		t.Errorf("got %q, want 67890", val)
	}
}

func TestLabelMappings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.SetLabelMapping("Finance", "Label_123")
	gmailID, err := store.GetLabelMapping("Finance")
	if err != nil {
		t.Fatalf("GetLabelMapping failed: %v", err)
	}
	if gmailID != "Label_123" {
		t.Errorf("got %q, want Label_123", gmailID)
	}
}

func TestStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.InsertMessage("msg1", "t1")
	store.InsertMessage("msg2", "t2")
	store.InsertMessage("msg3", "t3")
	store.MarkProcessing("msg2")
	store.MarkLabeled("msg2", "Finance")

	stats, _ := store.Stats()
	if stats.Pending != 2 {
		t.Errorf("pending=%d, want 2", stats.Pending)
	}
	if stats.Labeled != 1 {
		t.Errorf("labeled=%d, want 1", stats.Labeled)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -v`
Expected: FAIL — types and functions not defined

**Step 3: Write implementation**

```go
// internal/db/db.go
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const maxRetries = 3

type Store struct {
	db *sql.DB
}

type Message struct {
	ID          string
	ThreadID    string
	Status      string
	Label       sql.NullString
	Attempts    int
	CreatedAt   string
	ProcessedAt sql.NullString
}

type Stats struct {
	Pending int
	Labeled int
	Failed  int
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("creating db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	// Enable WAL mode for better concurrent read/write
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			thread_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			label TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			processed_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status)`,
		`CREATE TABLE IF NOT EXISTS state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS label_mappings (
			name TEXT PRIMARY KEY,
			gmail_id TEXT NOT NULL
		)`,
	}
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

func (s *Store) InsertMessage(id, threadID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO messages (id, thread_id) VALUES (?, ?)`,
		id, threadID,
	)
	return err
}

func (s *Store) PendingMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, thread_id, status, label, attempts, created_at, processed_at
		 FROM messages WHERE status = 'pending' ORDER BY created_at LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Status, &m.Label, &m.Attempts, &m.CreatedAt, &m.ProcessedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *Store) GetMessage(id string) (*Message, error) {
	var m Message
	err := s.db.QueryRow(
		`SELECT id, thread_id, status, label, attempts, created_at, processed_at
		 FROM messages WHERE id = ?`, id,
	).Scan(&m.ID, &m.ThreadID, &m.Status, &m.Label, &m.Attempts, &m.CreatedAt, &m.ProcessedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) MarkProcessing(id string) error {
	_, err := s.db.Exec(`UPDATE messages SET status = 'processing' WHERE id = ?`, id)
	return err
}

func (s *Store) MarkLabeled(id, label string) error {
	_, err := s.db.Exec(
		`UPDATE messages SET status = 'labeled', label = ?, processed_at = datetime('now') WHERE id = ?`,
		label, id,
	)
	return err
}

func (s *Store) MarkFailed(id string) error {
	_, err := s.db.Exec(
		`UPDATE messages SET
			attempts = attempts + 1,
			status = CASE WHEN attempts + 1 >= ? THEN 'failed' ELSE 'pending' END
		 WHERE id = ?`,
		maxRetries, id,
	)
	return err
}

func (s *Store) ResetProcessing() error {
	_, err := s.db.Exec(`UPDATE messages SET status = 'pending' WHERE status = 'processing'`)
	return err
}

func (s *Store) SetState(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?`,
		key, value, value,
	)
	return err
}

func (s *Store) GetState(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM state WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (s *Store) SetLabelMapping(name, gmailID string) error {
	_, err := s.db.Exec(
		`INSERT INTO label_mappings (name, gmail_id) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET gmail_id = ?`,
		name, gmailID, gmailID,
	)
	return err
}

func (s *Store) GetLabelMapping(name string) (string, error) {
	var gmailID string
	err := s.db.QueryRow(`SELECT gmail_id FROM label_mappings WHERE name = ?`, name).Scan(&gmailID)
	return gmailID, err
}

func (s *Store) AllLabelMappings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT name, gmail_id FROM label_mappings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mappings := make(map[string]string)
	for rows.Next() {
		var name, gmailID string
		if err := rows.Scan(&name, &gmailID); err != nil {
			return nil, err
		}
		mappings[name] = gmailID
	}
	return mappings, rows.Err()
}

func (s *Store) Stats() (*Stats, error) {
	var stats Stats
	err := s.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'labeled' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0)
		FROM messages`).Scan(&stats.Pending, &stats.Labeled, &stats.Failed)
	return &stats, err
}
```

**Step 4: Install dependency and run tests**

Run: `go get modernc.org/sqlite && go test ./internal/db/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat: add database package with SQLite queue and state management"
```

---

### Task 4: Logger Package

**Files:**
- Create: `internal/log/log.go`
- Create: `internal/log/log_test.go`

**Step 1: Write the failing test**

```go
// internal/log/log_test.go
package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	logger, err := New(path)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer logger.Close()

	logger.Info("hello %s", "world")
	logger.Error("something %s", "broke")

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "INFO") || !strings.Contains(content, "hello world") {
		t.Errorf("log missing INFO entry, got: %s", content)
	}
	if !strings.Contains(content, "ERROR") || !strings.Contains(content, "something broke") {
		t.Errorf("log missing ERROR entry, got: %s", content)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/log/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/log/log.go
package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type Logger struct {
	file   *os.File
	logger *log.Logger
}

func New(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &Logger{
		file:   f,
		logger: log.New(f, "", log.LstdFlags),
	}, nil
}

func (l *Logger) Info(format string, args ...any) {
	l.logger.Printf("INFO  "+format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.logger.Printf("ERROR "+format, args...)
}

func (l *Logger) Debug(format string, args ...any) {
	l.logger.Printf("DEBUG "+format, args...)
}

func (l *Logger) Close() error {
	return l.file.Close()
}

func (l *Logger) Path() string {
	return l.file.Name()
}

// Rotate checks file size and rotates if needed (>10MB).
func (l *Logger) Rotate(maxSize int64, keepCount int) error {
	info, err := l.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < maxSize {
		return nil
	}

	l.file.Close()
	path := l.file.Name()

	// Rotate existing files
	for i := keepCount - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", path, i)
		new := fmt.Sprintf("%s.%d", path, i+1)
		os.Rename(old, new)
	}
	os.Rename(path, path+".1")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	l.file = f
	l.logger = log.New(f, "", log.LstdFlags)
	return nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/log/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/log/
git commit -m "feat: add file logger with rotation support"
```

---

### Task 5: Gmail Auth Package

**Files:**
- Create: `internal/gmail/auth.go`
- Create: `internal/gmail/auth_test.go`

**Step 1: Write the failing test**

Note: OAuth flow involves browser interaction, so we test the token save/load and the HTTP server setup, not the full flow.

```go
// internal/gmail/auth_test.go
package gmail

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestSaveAndLoadToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	token := &oauth2.Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
	}

	if err := saveToken(path, token); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	// Verify file permissions
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("got permissions %o, want 0600", info.Mode().Perm())
	}

	loaded, err := loadToken(path)
	if err != nil {
		t.Fatalf("loadToken failed: %v", err)
	}
	if loaded.AccessToken != "access-123" {
		t.Errorf("got access token %q, want access-123", loaded.AccessToken)
	}
	if loaded.RefreshToken != "refresh-456" {
		t.Errorf("got refresh token %q, want refresh-456", loaded.RefreshToken)
	}
}

func TestOAuthConfig(t *testing.T) {
	cfg := oauthConfig("http://localhost:8080/callback")
	if cfg.ClientID == "" {
		t.Error("expected non-empty client ID")
	}
	if len(cfg.Scopes) != 2 {
		t.Errorf("got %d scopes, want 2", len(cfg.Scopes))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go get golang.org/x/oauth2 && go test ./internal/gmail/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/gmail/auth.go
package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailapi "google.golang.org/api/gmail/v1"
)

// TODO: Replace with actual published OAuth client credentials
const (
	clientID     = "YOUR_CLIENT_ID.apps.googleusercontent.com"
	clientSecret = "YOUR_CLIENT_SECRET"
)

func oauthConfig(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			gmailapi.GmailModifyScope,
			gmailapi.GmailLabelsScope,
		},
		Endpoint: google.Endpoint,
	}
}

// Authenticate runs the full OAuth flow: starts local server, opens browser,
// waits for callback, exchanges code for token, and saves it.
func Authenticate(credentialsPath string) (*oauth2.Token, error) {
	// Find available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("finding available port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)
	cfg := oauthConfig(redirectURL)

	// Channel to receive the auth code
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			fmt.Fprintln(w, "Error: no authorization code received.")
			return
		}
		codeCh <- code
		fmt.Fprintln(w, "Success! You can close this tab and return to the terminal.")
	})

	server := &http.Server{Addr: fmt.Sprintf("localhost:%d", port), Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Open browser
	authURL := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	openBrowser(authURL)

	fmt.Printf("If the browser didn't open, visit this URL:\n%s\n\n", authURL)

	// Wait for code or error
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		server.Close()
		return nil, err
	}

	server.Close()

	// Exchange code for token
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}

	if err := saveToken(credentialsPath, token); err != nil {
		return nil, fmt.Errorf("saving token: %w", err)
	}

	return token, nil
}

// TokenSource returns an oauth2.TokenSource from saved credentials.
// It auto-refreshes the token and saves the refreshed token back.
func TokenSource(credentialsPath string) (oauth2.TokenSource, error) {
	token, err := loadToken(credentialsPath)
	if err != nil {
		return nil, err
	}
	cfg := oauthConfig("")
	ts := cfg.TokenSource(context.Background(), token)

	return &savingTokenSource{
		src:  ts,
		path: credentialsPath,
		prev: token,
	}, nil
}

type savingTokenSource struct {
	src  oauth2.TokenSource
	path string
	prev *oauth2.Token
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	// If token was refreshed, save it
	if token.AccessToken != s.prev.AccessToken {
		saveToken(s.path, token)
		s.prev = token
	}
	return token, nil
}

func saveToken(path string, token *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}
```

**Step 4: Install deps and run tests**

Run: `go get google.golang.org/api/gmail/v1 golang.org/x/oauth2 && go test ./internal/gmail/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/gmail/auth.go internal/gmail/auth_test.go
git commit -m "feat: add Gmail OAuth auth flow with token save/load"
```

---

### Task 6: Gmail Client Package

**Files:**
- Create: `internal/gmail/client.go`
- Create: `internal/gmail/client_test.go`

**Step 1: Write the failing test**

The Gmail client wraps the Google API. We define an interface for testability and test the extraction logic.

```go
// internal/gmail/client_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/gmail/ -v`
Expected: FAIL — functions not defined

**Step 3: Write implementation**

```go
// internal/gmail/client.go
package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type Client struct {
	svc *gmail.Service
}

type EmailData struct {
	ID      string
	From    string
	Subject string
	Body    string
}

type MessageHeader struct {
	Name  string
	Value string
}

func NewClient(ctx context.Context, ts oauth2.TokenSource) (*Client, error) {
	svc, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("creating gmail service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// GetNewMessageIDs returns message IDs added to inbox since the given historyId.
// Returns the new historyId to use for the next poll.
func (c *Client) GetNewMessageIDs(ctx context.Context, historyID uint64) ([]struct{ ID, ThreadID string }, uint64, error) {
	resp, err := c.svc.Users.History.List("me").
		StartHistoryId(historyID).
		LabelId("INBOX").
		HistoryTypes("messageAdded").
		Context(ctx).
		Do()
	if err != nil {
		return nil, 0, fmt.Errorf("listing history: %w", err)
	}

	var messages []struct{ ID, ThreadID string }
	seen := make(map[string]bool)
	for _, h := range resp.History {
		for _, m := range h.MessagesAdded {
			if !seen[m.Message.Id] {
				seen[m.Message.Id] = true
				messages = append(messages, struct{ ID, ThreadID string }{
					ID:       m.Message.Id,
					ThreadID: m.Message.ThreadId,
				})
			}
		}
	}

	return messages, resp.HistoryId, nil
}

// GetEmail fetches an email and extracts relevant data for classification.
func (c *Client) GetEmail(ctx context.Context, messageID string) (*EmailData, error) {
	msg, err := c.svc.Users.Messages.Get("me", messageID).
		Format("full").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("getting message %s: %w", messageID, err)
	}

	var headers []MessageHeader
	for _, h := range msg.Payload.Headers {
		headers = append(headers, MessageHeader{Name: h.Name, Value: h.Value})
	}

	data := extractEmailHeaders(headers)
	data.ID = messageID
	data.Body = truncateText(extractBody(msg.Payload), 500)

	return data, nil
}

// ApplyLabel applies a Gmail label to a message.
func (c *Client) ApplyLabel(ctx context.Context, messageID, labelID string) error {
	_, err := c.svc.Users.Messages.Modify("me", messageID, &gmail.ModifyMessageRequest{
		AddLabelIds: []string{labelID},
	}).Context(ctx).Do()
	return err
}

// CreateLabel creates a Gmail label and returns its ID. If it already exists, returns the existing ID.
func (c *Client) CreateLabel(ctx context.Context, name string) (string, error) {
	label, err := c.svc.Users.Labels.Create("me", &gmail.Label{
		Name:                    name,
		LabelListVisibility:     "labelShow",
		MessageListVisibility:   "show",
	}).Context(ctx).Do()
	if err != nil {
		// Check if label already exists by listing all labels
		if existingID, findErr := c.findLabelByName(ctx, name); findErr == nil {
			return existingID, nil
		}
		return "", fmt.Errorf("creating label %q: %w", name, err)
	}
	return label.Id, nil
}

func (c *Client) findLabelByName(ctx context.Context, name string) (string, error) {
	resp, err := c.svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	for _, l := range resp.Labels {
		if l.Name == name {
			return l.Id, nil
		}
	}
	return "", fmt.Errorf("label %q not found", name)
}

// GetProfile returns the user's email and current historyId.
func (c *Client) GetProfile(ctx context.Context) (email string, historyID uint64, err error) {
	profile, err := c.svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return "", 0, err
	}
	return profile.EmailAddress, profile.HistoryId, nil
}

// ListRecentMessages returns recent message IDs from the inbox.
func (c *Client) ListRecentMessages(ctx context.Context, maxResults int64) ([]struct{ ID, ThreadID string }, error) {
	resp, err := c.svc.Users.Messages.List("me").
		LabelIds("INBOX").
		MaxResults(maxResults).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	var msgs []struct{ ID, ThreadID string }
	for _, m := range resp.Messages {
		msgs = append(msgs, struct{ ID, ThreadID string }{ID: m.Id, ThreadID: m.ThreadId})
	}
	return msgs, nil
}

func extractEmailHeaders(headers []MessageHeader) *EmailData {
	data := &EmailData{}
	for _, h := range headers {
		switch strings.ToLower(h.Name) {
		case "from":
			data.From = h.Value
		case "subject":
			data.Subject = h.Value
		}
	}
	return data
}

func extractBody(payload *gmail.MessagePart) string {
	// Try to find plain text part
	if payload.MimeType == "text/plain" && payload.Body != nil && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}

	// Recurse into parts
	for _, part := range payload.Parts {
		if body := extractBody(part); body != "" {
			return body
		}
	}

	// Fall back to snippet if no plain text found
	return ""
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
```

**Step 4: Run tests**

Run: `go test ./internal/gmail/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/gmail/client.go internal/gmail/client_test.go
git commit -m "feat: add Gmail client with history polling, email fetch, label management"
```

---

### Task 7: AI Providers Package

**Files:**
- Create: `internal/ai/providers.go`
- Create: `internal/ai/providers_test.go`

**Step 1: Write the failing test**

```go
// internal/ai/providers_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/ai/providers.go
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
```

**Step 4: Run tests**

Run: `go test ./internal/ai/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/ai/providers.go internal/ai/providers_test.go
git commit -m "feat: add AI provider registry with OpenAI, Ollama, DeepSeek, Groq"
```

---

### Task 8: AI Classifier Package

**Files:**
- Create: `internal/ai/classifier.go`
- Create: `internal/ai/classifier_test.go`

**Step 1: Write the failing test**

```go
// internal/ai/classifier_test.go
package ai

import (
	"testing"

	"github.com/pankajbeniwal/labelr/internal/config"
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/ai/classifier.go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/pankajbeniwal/labelr/internal/config"
)

type Classifier struct {
	client *openai.Client
	model  string
	labels []config.Label
}

type ClassificationResult struct {
	Label string `json:"label"`
}

func NewClassifier(apiKey, baseURL, model string, labels []config.Label) *Classifier {
	opts := []openai.Option{}
	if apiKey != "" {
		opts = append(opts, openai.WithAPIKey(apiKey))
	} else {
		// For Ollama or no-auth providers
		opts = append(opts, openai.WithAPIKey("ollama"))
	}
	if baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}

	client := openai.NewClient(opts...)

	return &Classifier{
		client: client,
		model:  model,
		labels: labels,
	}
}

func (c *Classifier) Classify(ctx context.Context, from, subject, body string) (string, error) {
	prompt := buildPrompt(from, subject, body, c.labels)
	schema := buildResponseSchema(c.labels)

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("marshaling schema: %w", err)
	}

	var schemaMap map[string]any
	json.Unmarshal(schemaBytes, &schemaMap)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are an email classifier. You must respond with valid JSON only."),
			openai.UserMessage(prompt),
		},
		Model:           c.model,
		MaxOutputTokens: openai.Int(50),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "email_classification",
					Schema: schemaMap,
					Strict: openai.Bool(true),
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("AI classification failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from AI")
	}

	var result ClassificationResult
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
		return "", fmt.Errorf("parsing AI response: %w", err)
	}

	if result.Label == "" {
		return "", fmt.Errorf("AI returned empty label")
	}

	return result.Label, nil
}

func buildPrompt(from, subject, body string, labels []config.Label) string {
	var sb strings.Builder
	sb.WriteString("Classify this email into exactly one of the provided labels.\n\n")
	sb.WriteString("Email:\n")
	sb.WriteString(fmt.Sprintf("- From: %s\n", from))
	sb.WriteString(fmt.Sprintf("- Subject: %s\n", subject))
	if body != "" {
		sb.WriteString(fmt.Sprintf("- Body preview: %s\n", body))
	}
	sb.WriteString("\nAvailable labels:\n")
	for _, l := range labels {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", l.Name, l.Description))
	}
	sb.WriteString("\nRespond with JSON: {\"label\": \"<label_name>\"}")
	return sb.String()
}

func buildResponseSchema(labels []config.Label) map[string]any {
	labelNames := make([]any, len(labels))
	for i, l := range labels {
		labelNames[i] = l.Name
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"label": map[string]any{
				"type": "string",
				"enum": labelNames,
			},
		},
		"required":             []string{"label"},
		"additionalProperties": false,
	}
}
```

**Step 4: Install dependency and run tests**

Run: `go get github.com/openai/openai-go/v3 && go test ./internal/ai/ -v`
Expected: All PASS (only testing prompt/schema building, not actual API calls)

**Step 5: Commit**

```bash
git add internal/ai/classifier.go internal/ai/classifier_test.go
git commit -m "feat: add AI classifier with structured output and prompt building"
```

---

### Task 9: Daemon - Poller

**Files:**
- Create: `internal/daemon/poller.go`
- Create: `internal/daemon/poller_test.go`

**Step 1: Write the failing test**

```go
// internal/daemon/poller_test.go
package daemon

import (
	"context"
	"testing"
	"path/filepath"

	"github.com/pankajbeniwal/labelr/internal/db"
)

// mockGmailPoller implements the GmailPoller interface for testing
type mockGmailPoller struct {
	messages []struct{ ID, ThreadID string }
	newHistoryID uint64
	err          error
}

func (m *mockGmailPoller) GetNewMessageIDs(ctx context.Context, historyID uint64) ([]struct{ ID, ThreadID string }, uint64, error) {
	return m.messages, m.newHistoryID, m.err
}

func TestPollerProcessesNewMessages(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	defer store.Close()
	store.SetState("history_id", "100")

	mock := &mockGmailPoller{
		messages: []struct{ ID, ThreadID string }{
			{ID: "msg1", ThreadID: "t1"},
			{ID: "msg2", ThreadID: "t2"},
		},
		newHistoryID: 200,
	}

	poller := NewPoller(store, mock, nil)
	err := poller.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll failed: %v", err)
	}

	msgs, _ := store.PendingMessages(10)
	if len(msgs) != 2 {
		t.Errorf("got %d pending messages, want 2", len(msgs))
	}

	historyID, _ := store.GetState("history_id")
	if historyID != "200" {
		t.Errorf("got historyID %s, want 200", historyID)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/daemon/poller.go
package daemon

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pankajbeniwal/labelr/internal/db"
	applog "github.com/pankajbeniwal/labelr/internal/log"
)

// GmailPoller is the interface for fetching new message IDs from Gmail.
type GmailPoller interface {
	GetNewMessageIDs(ctx context.Context, historyID uint64) ([]struct{ ID, ThreadID string }, uint64, error)
}

type Poller struct {
	store  *db.Store
	gmail  GmailPoller
	logger *applog.Logger
}

func NewPoller(store *db.Store, gmail GmailPoller, logger *applog.Logger) *Poller {
	return &Poller{store: store, gmail: gmail, logger: logger}
}

func (p *Poller) Poll(ctx context.Context) error {
	historyIDStr, err := p.store.GetState("history_id")
	if err != nil {
		return fmt.Errorf("getting history_id: %w", err)
	}

	historyID, err := strconv.ParseUint(historyIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parsing history_id: %w", err)
	}

	messages, newHistoryID, err := p.gmail.GetNewMessageIDs(ctx, historyID)
	if err != nil {
		return fmt.Errorf("fetching new messages: %w", err)
	}

	for _, msg := range messages {
		if err := p.store.InsertMessage(msg.ID, msg.ThreadID); err != nil {
			if p.logger != nil {
				p.logger.Error("inserting message %s: %v", msg.ID, err)
			}
		}
	}

	if newHistoryID > 0 {
		p.store.SetState("history_id", strconv.FormatUint(newHistoryID, 10))
	}

	if p.logger != nil && len(messages) > 0 {
		p.logger.Info("polled %d new messages", len(messages))
	}

	return nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/daemon/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/daemon/
git commit -m "feat: add poller that fetches new Gmail messages into queue"
```

---

### Task 10: Daemon - Worker

**Files:**
- Create: `internal/daemon/worker.go`
- Create: `internal/daemon/worker_test.go`

**Step 1: Write the failing test**

```go
// internal/daemon/worker_test.go
package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/gmail"
)

type mockClassifier struct {
	label string
	err   error
}

func (m *mockClassifier) Classify(ctx context.Context, from, subject, body string) (string, error) {
	return m.label, m.err
}

type mockEmailFetcher struct {
	email *gmail.EmailData
	err   error
}

func (m *mockEmailFetcher) GetEmail(ctx context.Context, messageID string) (*gmail.EmailData, error) {
	return m.email, m.err
}

type mockLabelApplier struct {
	applied []struct{ MsgID, LabelID string }
	err     error
}

func (m *mockLabelApplier) ApplyLabel(ctx context.Context, messageID, labelID string) error {
	m.applied = append(m.applied, struct{ MsgID, LabelID string }{messageID, labelID})
	return m.err
}

func TestWorkerProcessesMessage(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	defer store.Close()

	store.InsertMessage("msg1", "t1")
	store.SetLabelMapping("Finance", "Label_123")

	fetcher := &mockEmailFetcher{
		email: &gmail.EmailData{
			ID: "msg1", From: "bank@example.com",
			Subject: "Your statement", Body: "Monthly statement",
		},
	}
	classifier := &mockClassifier{label: "Finance"}
	applier := &mockLabelApplier{}

	w := NewWorker(store, fetcher, classifier, applier, nil)
	processed, err := w.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("ProcessOne failed: %v", err)
	}
	if !processed {
		t.Error("expected processed=true")
	}
	if len(applier.applied) != 1 {
		t.Fatalf("expected 1 label applied, got %d", len(applier.applied))
	}
	if applier.applied[0].LabelID != "Label_123" {
		t.Errorf("got labelID=%q, want Label_123", applier.applied[0].LabelID)
	}

	msg, _ := store.GetMessage("msg1")
	if msg.Status != "labeled" {
		t.Errorf("got status=%q, want labeled", msg.Status)
	}
}

func TestWorkerHandlesClassificationError(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	defer store.Close()

	store.InsertMessage("msg1", "t1")

	fetcher := &mockEmailFetcher{
		email: &gmail.EmailData{ID: "msg1", From: "a@b.com", Subject: "test"},
	}
	classifier := &mockClassifier{err: fmt.Errorf("AI down")}
	applier := &mockLabelApplier{}

	w := NewWorker(store, fetcher, classifier, applier, nil)
	w.ProcessOne(context.Background())

	msg, _ := store.GetMessage("msg1")
	if msg.Attempts != 1 {
		t.Errorf("got attempts=%d, want 1", msg.Attempts)
	}
}

func TestWorkerNoPendingMessages(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	defer store.Close()

	w := NewWorker(store, nil, nil, nil, nil)
	processed, err := w.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed {
		t.Error("expected processed=false when queue is empty")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -v`
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/daemon/worker.go
package daemon

import (
	"context"
	"fmt"

	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/gmail"
	applog "github.com/pankajbeniwal/labelr/internal/log"
)

type EmailClassifier interface {
	Classify(ctx context.Context, from, subject, body string) (string, error)
}

type EmailFetcher interface {
	GetEmail(ctx context.Context, messageID string) (*gmail.EmailData, error)
}

type LabelApplier interface {
	ApplyLabel(ctx context.Context, messageID, labelID string) error
}

type Worker struct {
	store      *db.Store
	fetcher    EmailFetcher
	classifier EmailClassifier
	applier    LabelApplier
	logger     *applog.Logger
}

func NewWorker(store *db.Store, fetcher EmailFetcher, classifier EmailClassifier, applier LabelApplier, logger *applog.Logger) *Worker {
	return &Worker{
		store:      store,
		fetcher:    fetcher,
		classifier: classifier,
		applier:    applier,
		logger:     logger,
	}
}

// ProcessOne processes a single pending message. Returns true if a message was processed.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	msgs, err := w.store.PendingMessages(1)
	if err != nil {
		return false, fmt.Errorf("getting pending messages: %w", err)
	}
	if len(msgs) == 0 {
		return false, nil
	}

	msg := msgs[0]
	w.store.MarkProcessing(msg.ID)

	// Fetch email
	email, err := w.fetcher.GetEmail(ctx, msg.ID)
	if err != nil {
		w.store.MarkFailed(msg.ID)
		w.logError("fetching email %s: %v", msg.ID, err)
		return true, nil
	}

	// Classify
	label, err := w.classifier.Classify(ctx, email.From, email.Subject, email.Body)
	if err != nil {
		w.store.MarkFailed(msg.ID)
		w.logError("classifying email %s: %v", msg.ID, err)
		return true, nil
	}

	// Get Gmail label ID
	gmailID, err := w.store.GetLabelMapping(label)
	if err != nil {
		w.store.MarkFailed(msg.ID)
		w.logError("getting label mapping for %q: %v", label, err)
		return true, nil
	}

	// Apply label
	if err := w.applier.ApplyLabel(ctx, msg.ID, gmailID); err != nil {
		w.store.MarkFailed(msg.ID)
		w.logError("applying label to %s: %v", msg.ID, err)
		return true, nil
	}

	w.store.MarkLabeled(msg.ID, label)
	w.logInfo("labeled %s as %q", msg.ID, label)
	return true, nil
}

func (w *Worker) logInfo(format string, args ...any) {
	if w.logger != nil {
		w.logger.Info(format, args...)
	}
}

func (w *Worker) logError(format string, args ...any) {
	if w.logger != nil {
		w.logger.Error(format, args...)
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/daemon/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/daemon/worker.go internal/daemon/worker_test.go
git commit -m "feat: add worker that classifies and labels emails from queue"
```

---

### Task 11: Daemon Orchestrator

**Files:**
- Create: `internal/daemon/daemon.go`

**Step 1: Write implementation**

This orchestrates the poller and worker loops with graceful shutdown. Hard to unit test the loop itself, but the components it uses are well-tested.

```go
// internal/daemon/daemon.go
package daemon

import (
	"context"
	"time"

	"github.com/pankajbeniwal/labelr/internal/db"
	applog "github.com/pankajbeniwal/labelr/internal/log"
)

type Daemon struct {
	store    *db.Store
	poller   *Poller
	worker   *Worker
	logger   *applog.Logger
	pollInterval time.Duration
}

func New(store *db.Store, poller *Poller, worker *Worker, logger *applog.Logger, pollInterval time.Duration) *Daemon {
	return &Daemon{
		store:        store,
		poller:       poller,
		worker:       worker,
		logger:       logger,
		pollInterval: pollInterval,
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	// Reset any interrupted processing messages
	if err := d.store.ResetProcessing(); err != nil {
		d.logger.Error("resetting processing messages: %v", err)
	}

	d.logger.Info("daemon started, polling every %s", d.pollInterval)

	// Start poller in background
	go d.pollLoop(ctx)

	// Run worker in foreground
	d.workerLoop(ctx)

	d.logger.Info("daemon stopped")
	return nil
}

func (d *Daemon) pollLoop(ctx context.Context) {
	// Poll immediately on start
	if err := d.poller.Poll(ctx); err != nil {
		d.logger.Error("poll error: %v", err)
	}

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.poller.Poll(ctx); err != nil {
				d.logger.Error("poll error: %v", err)
			}
		}
	}
}

func (d *Daemon) workerLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			processed, err := d.worker.ProcessOne(ctx)
			if err != nil {
				d.logger.Error("worker error: %v", err)
			}
			if !processed {
				// No messages to process, sleep briefly
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}
	}
}
```

**Step 2: Commit**

```bash
git add internal/daemon/daemon.go
git commit -m "feat: add daemon orchestrator with poller and worker loops"
```

---

### Task 12: Service Management Package

**Files:**
- Create: `internal/service/service.go`
- Create: `internal/service/launchd.go`
- Create: `internal/service/systemd.go`
- Create: `internal/service/taskscheduler.go`
- Create: `internal/service/service_test.go`

**Step 1: Write the failing test**

```go
// internal/service/service_test.go
package service

import (
	"runtime"
	"testing"
)

func TestDetectManager(t *testing.T) {
	mgr := Detect()
	if mgr == nil {
		t.Fatal("Detect() returned nil")
	}

	switch runtime.GOOS {
	case "darwin":
		if _, ok := mgr.(*LaunchdManager); !ok {
			t.Error("expected LaunchdManager on darwin")
		}
	case "linux":
		if _, ok := mgr.(*SystemdManager); !ok {
			t.Error("expected SystemdManager on linux")
		}
	case "windows":
		if _, ok := mgr.(*TaskSchedulerManager); !ok {
			t.Error("expected TaskSchedulerManager on windows")
		}
	}
}

func TestLaunchdPlistContent(t *testing.T) {
	mgr := &LaunchdManager{}
	content := mgr.plistContent("/usr/local/bin/labelr")
	if content == "" {
		t.Error("plist content is empty")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -v`
Expected: FAIL

**Step 3: Write service interface and detection**

```go
// internal/service/service.go
package service

import "runtime"

type Manager interface {
	Install(binaryPath string) error
	Uninstall() error
	Start() error
	Stop() error
	IsRunning() (bool, error)
}

func Detect() Manager {
	switch runtime.GOOS {
	case "darwin":
		return &LaunchdManager{}
	case "linux":
		return &SystemdManager{}
	case "windows":
		return &TaskSchedulerManager{}
	default:
		return nil
	}
}
```

**Step 4: Write launchd implementation**

```go
// internal/service/launchd.go
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	launchdLabel = "com.labelr.daemon"
)

type LaunchdManager struct{}

func (m *LaunchdManager) plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func (m *LaunchdManager) plistContent(binaryPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/tmp/labelr-stderr.log</string>
</dict>
</plist>`, launchdLabel, binaryPath)
}

func (m *LaunchdManager) Install(binaryPath string) error {
	content := m.plistContent(binaryPath)
	path := m.plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func (m *LaunchdManager) Uninstall() error {
	m.Stop()
	return os.Remove(m.plistPath())
}

func (m *LaunchdManager) Start() error {
	return exec.Command("launchctl", "load", m.plistPath()).Run()
}

func (m *LaunchdManager) Stop() error {
	return exec.Command("launchctl", "unload", m.plistPath()).Run()
}

func (m *LaunchdManager) IsRunning() (bool, error) {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), launchdLabel), nil
}
```

**Step 5: Write systemd implementation**

```go
// internal/service/systemd.go
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdServiceName = "labelr.service"

type SystemdManager struct{}

func (m *SystemdManager) unitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", systemdServiceName)
}

func (m *SystemdManager) unitContent(binaryPath string) string {
	return fmt.Sprintf(`[Unit]
Description=labelr - AI Gmail Labeler
After=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, binaryPath)
}

func (m *SystemdManager) Install(binaryPath string) error {
	content := m.unitContent(binaryPath)
	path := m.unitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

func (m *SystemdManager) Uninstall() error {
	exec.Command("systemctl", "--user", "disable", "labelr").Run()
	m.Stop()
	return os.Remove(m.unitPath())
}

func (m *SystemdManager) Start() error {
	return exec.Command("systemctl", "--user", "enable", "--now", "labelr").Run()
}

func (m *SystemdManager) Stop() error {
	return exec.Command("systemctl", "--user", "stop", "labelr").Run()
}

func (m *SystemdManager) IsRunning() (bool, error) {
	out, err := exec.Command("systemctl", "--user", "is-active", "labelr").Output()
	if err != nil {
		return false, nil // not running
	}
	return strings.TrimSpace(string(out)) == "active", nil
}
```

**Step 6: Write Windows Task Scheduler implementation**

```go
// internal/service/taskscheduler.go
package service

import (
	"os/exec"
	"strings"
)

const taskName = "labelr"

type TaskSchedulerManager struct{}

func (m *TaskSchedulerManager) Install(binaryPath string) error {
	return exec.Command("schtasks", "/create",
		"/tn", taskName,
		"/tr", binaryPath+" daemon",
		"/sc", "onlogon",
		"/rl", "LIMITED",
		"/f",
	).Run()
}

func (m *TaskSchedulerManager) Uninstall() error {
	m.Stop()
	return exec.Command("schtasks", "/delete", "/tn", taskName, "/f").Run()
}

func (m *TaskSchedulerManager) Start() error {
	return exec.Command("schtasks", "/run", "/tn", taskName).Run()
}

func (m *TaskSchedulerManager) Stop() error {
	return exec.Command("schtasks", "/end", "/tn", taskName).Run()
}

func (m *TaskSchedulerManager) IsRunning() (bool, error) {
	out, err := exec.Command("schtasks", "/query", "/tn", taskName, "/fo", "CSV", "/nh").Output()
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(out), "Running"), nil
}
```

**Step 7: Run tests**

Run: `go test ./internal/service/ -v`
Expected: All PASS

**Step 8: Commit**

```bash
git add internal/service/
git commit -m "feat: add cross-platform service management (launchd, systemd, Task Scheduler)"
```

---

### Task 13: CLI - Init Command

**Files:**
- Create: `internal/cli/init.go`
- Modify: `cmd/labelr/main.go`

**Step 1: Write the init command**

This is interactive (bubbletea), so testing is done manually. Focus on wiring the pieces together.

```go
// internal/cli/init.go
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/spf13/cobra"
)

func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive first-time setup",
		Long:  "Set up Gmail authentication, AI provider, and label configuration.",
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Welcome to labelr! Let's get you set up.\n")

	// Ensure config directory exists
	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Step 1: Gmail OAuth
	fmt.Println("Step 1: Connect your Gmail account")
	fmt.Println("A browser window will open for you to sign in with Google.\n")

	var proceed bool
	huh.NewConfirm().
		Title("Ready to authenticate with Gmail?").
		Value(&proceed).
		Run()

	if !proceed {
		return fmt.Errorf("setup cancelled")
	}

	token, err := gmailpkg.Authenticate(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("Gmail authentication failed: %w", err)
	}
	_ = token

	// Get user email
	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating token source: %w", err)
	}
	client, err := gmailpkg.NewClient(context.Background(), ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}
	email, historyID, err := client.GetProfile(context.Background())
	if err != nil {
		return fmt.Errorf("getting profile: %w", err)
	}
	fmt.Printf("Connected as: %s\n\n", email)

	// Step 2: AI Provider
	fmt.Println("Step 2: Choose your AI provider")

	providerNames := ai.ProviderNames()
	var selectedProvider string
	huh.NewSelect[string]().
		Title("Which AI provider?").
		Options(huh.NewOptions(providerNames...)...).
		Value(&selectedProvider).
		Run()

	provider, _ := ai.GetProvider(selectedProvider)

	// Step 3: Model
	var model string
	huh.NewInput().
		Title("Which model? (e.g., gpt-4o-mini, llama3, deepseek-chat)").
		Value(&model).
		Run()

	// Step 4: API Key
	var apiKey string
	if provider.EnvKey != "" {
		// Check env first
		if envVal := os.Getenv(provider.EnvKey); envVal != "" {
			fmt.Printf("Found API key in $%s\n", provider.EnvKey)
			apiKey = envVal
		} else {
			huh.NewInput().
				Title(fmt.Sprintf("Enter your %s API key:", provider.Name)).
				Value(&apiKey).
				EchoMode(huh.EchoModePassword).
				Run()
		}
	}

	// Step 5: Labels
	fmt.Println("\nStep 3: Configure labels")
	labels := config.DefaultLabels()
	fmt.Println("Default labels:")
	for _, l := range labels {
		fmt.Printf("  - %s: %s\n", l.Name, l.Description)
	}

	var useDefaults bool
	huh.NewConfirm().
		Title("Use default labels?").
		Value(&useDefaults).
		Run()

	if !useDefaults {
		// TODO: Interactive label editor with bubbletea
		fmt.Println("Custom label editing coming soon. Using defaults for now.")
	}

	// Save config
	cfg := &config.Config{
		Gmail:        config.GmailConfig{Email: email},
		AI:           config.AIConfig{Provider: selectedProvider, Model: model, APIKey: apiKey, BaseURL: provider.BaseURL},
		Labels:       labels,
		PollInterval: 60,
	}
	if err := config.Save(config.DefaultPath(), cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Create labels in Gmail
	fmt.Println("\nCreating Gmail labels...")
	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	for _, l := range labels {
		gmailID, err := client.CreateLabel(context.Background(), l.Name)
		if err != nil {
			fmt.Printf("  Warning: could not create label %q: %v\n", l.Name, err)
			continue
		}
		store.SetLabelMapping(l.Name, gmailID)
		fmt.Printf("  Created: %s\n", l.Name)
	}

	// Store initial historyId
	store.SetState("history_id", fmt.Sprintf("%d", historyID))

	// Offer test run
	var testRun bool
	huh.NewConfirm().
		Title("Label your 10 most recent emails to test?").
		Value(&testRun).
		Run()

	if testRun {
		fmt.Println("Fetching recent emails...")
		msgs, err := client.ListRecentMessages(context.Background(), 10)
		if err != nil {
			fmt.Printf("Warning: could not fetch recent messages: %v\n", err)
		} else {
			for _, m := range msgs {
				store.InsertMessage(m.ID, m.ThreadID)
			}
			fmt.Printf("Added %d emails to queue. They'll be labeled when you run 'labelr start'.\n", len(msgs))
		}
	}

	fmt.Println("\nSetup complete! Run 'labelr start' to begin labeling emails.")
	return nil
}
```

**Step 2: Update main.go to register commands**

```go
// cmd/labelr/main.go
package main

import (
	"fmt"
	"os"

	"github.com/pankajbeniwal/labelr/internal/cli"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "labelr",
		Short:   "AI-powered Gmail labeler",
		Long:    "labelr automatically classifies and labels your Gmail emails using AI.",
		Version: version,
	}

	rootCmd.AddCommand(cli.NewInitCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

**Step 3: Install huh and verify build**

Run: `go get github.com/charmbracelet/huh && make build`
Expected: Binary builds successfully

**Step 4: Commit**

```bash
git add internal/cli/init.go cmd/labelr/main.go
git commit -m "feat: add interactive init command with Gmail auth, provider selection, labels"
```

---

### Task 14: CLI - Daemon Command (Hidden)

**Files:**
- Create: `internal/cli/daemon.go`
- Modify: `cmd/labelr/main.go`

**Step 1: Write the daemon command**

```go
// internal/cli/daemon.go
package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/daemon"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	applog "github.com/pankajbeniwal/labelr/internal/log"
	"github.com/spf13/cobra"
)

func NewDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "daemon",
		Short:  "Run the daemon in foreground",
		Hidden: true,
		RunE:   runDaemon,
	}
}

func runDaemon(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("loading config (run 'labelr init' first): %w", err)
	}

	// Set up logger
	logger, err := applog.New(config.LogPath())
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer logger.Close()

	// Open database
	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Create Gmail client
	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating Gmail token source: %w", err)
	}
	gmailClient, err := gmailpkg.NewClient(cmd.Context(), ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}

	// Create AI classifier
	apiKey := cfg.ResolveAPIKey()
	classifier := ai.NewClassifier(apiKey, cfg.AI.BaseURL, cfg.AI.Model, cfg.Labels)

	// Create daemon components
	poller := daemon.NewPoller(store, gmailClient, logger)
	worker := daemon.NewWorker(store, gmailClient, classifier, gmailClient, logger)

	d := daemon.New(store, poller, worker, logger, time.Duration(cfg.PollInterval)*time.Second)

	// Run with graceful shutdown
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return d.Run(ctx)
}
```

**Step 2: Register in main.go**

Add `rootCmd.AddCommand(cli.NewDaemonCmd())` after the init command in main.go.

**Step 3: Verify build**

Run: `make build`
Expected: Builds successfully

**Step 4: Commit**

```bash
git add internal/cli/daemon.go cmd/labelr/main.go
git commit -m "feat: add hidden daemon command that runs poller and worker loops"
```

---

### Task 15: CLI - Start and Stop Commands

**Files:**
- Create: `internal/cli/start.go`
- Create: `internal/cli/stop.go`
- Modify: `cmd/labelr/main.go`

**Step 1: Write start command**

```go
// internal/cli/start.go
package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Install and start background service",
		RunE:  runStart,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check config exists
	if _, err := os.Stat(config.DefaultPath()); os.IsNotExist(err) {
		return fmt.Errorf("no config found. Run 'labelr init' first")
	}

	// Find our binary path
	binaryPath, err := exec.LookPath("labelr")
	if err != nil {
		// Fall back to current executable
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("finding labelr binary: %w", err)
		}
	}

	mgr := service.Detect()
	if mgr == nil {
		return fmt.Errorf("unsupported operating system")
	}

	// Install the service
	fmt.Println("Installing background service...")
	if err := mgr.Install(binaryPath); err != nil {
		return fmt.Errorf("installing service: %w", err)
	}

	// Start it
	fmt.Println("Starting labelr daemon...")
	if err := mgr.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	fmt.Println("labelr is now running in the background.")
	fmt.Println("Use 'labelr status' to check on it, or 'labelr stop' to stop it.")
	return nil
}
```

**Step 2: Write stop command**

```go
// internal/cli/stop.go
package cli

import (
	"fmt"

	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the background service",
		RunE:  runStop,
	}
}

func runStop(cmd *cobra.Command, args []string) error {
	mgr := service.Detect()
	if mgr == nil {
		return fmt.Errorf("unsupported operating system")
	}

	fmt.Println("Stopping labelr daemon...")
	if err := mgr.Stop(); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}

	fmt.Println("labelr daemon stopped.")
	return nil
}
```

**Step 3: Register both commands in main.go**

Add `rootCmd.AddCommand(cli.NewStartCmd(), cli.NewStopCmd())` in main.go.

**Step 4: Verify build**

Run: `make build`
Expected: Builds successfully

**Step 5: Commit**

```bash
git add internal/cli/start.go internal/cli/stop.go cmd/labelr/main.go
git commit -m "feat: add start and stop commands for daemon service management"
```

---

### Task 16: CLI - Status, Logs, Config, Sync, Uninstall Commands

**Files:**
- Create: `internal/cli/status.go`
- Create: `internal/cli/logs.go`
- Create: `internal/cli/config_cmd.go`
- Create: `internal/cli/sync.go`
- Create: `internal/cli/uninstall.go`
- Modify: `cmd/labelr/main.go`

**Step 1: Write status command**

```go
// internal/cli/status.go
package cli

import (
	"fmt"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status and queue stats",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Check running status
	mgr := service.Detect()
	running := false
	if mgr != nil {
		running, _ = mgr.IsRunning()
	}

	if running {
		fmt.Println("Status:    running")
	} else {
		fmt.Println("Status:    stopped")
	}

	// Load config for provider info
	cfg, err := config.Load(config.DefaultPath())
	if err == nil {
		fmt.Printf("Provider:  %s / %s\n", cfg.AI.Provider, cfg.AI.Model)
		fmt.Printf("Account:   %s\n", cfg.Gmail.Email)
	}

	// Queue stats
	store, err := db.Open(config.DBPath())
	if err == nil {
		defer store.Close()
		stats, err := store.Stats()
		if err == nil {
			fmt.Printf("\nQueue:\n")
			fmt.Printf("  Pending:  %d\n", stats.Pending)
			fmt.Printf("  Labeled:  %d\n", stats.Labeled)
			fmt.Printf("  Failed:   %d\n", stats.Failed)
		}

		if lastPoll, err := store.GetState("last_poll_time"); err == nil {
			fmt.Printf("\nLast poll: %s\n", lastPoll)
		}
	}

	return nil
}
```

**Step 2: Write logs command**

```go
// internal/cli/logs.go
package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/spf13/cobra"
)

func NewLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Tail the daemon log file",
		RunE:  runLogs,
	}
}

func runLogs(cmd *cobra.Command, args []string) error {
	logPath := config.LogPath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s", logPath)
	}

	tailCmd := exec.Command("tail", "-f", logPath)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr
	return tailCmd.Run()
}
```

**Step 3: Write sync command**

```go
// internal/cli/sync.go
package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/spf13/cobra"
)

func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "One-time backlog scan",
		Long:  "Fetch and queue recent emails for labeling. Example: labelr sync --last 7d",
		RunE:  runSync,
	}
	cmd.Flags().String("last", "7d", "How far back to sync (e.g., 1d, 7d, 30d)")
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	lastStr, _ := cmd.Flags().GetString("last")
	duration, err := parseDuration(lastStr)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", lastStr, err)
	}

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	_ = cfg

	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating token source: %w", err)
	}

	ctx := context.Background()
	client, err := gmailpkg.NewClient(ctx, ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}

	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Estimate: ~50 emails per day
	estimate := int64(duration.Hours()/24) * 50
	if estimate < 10 {
		estimate = 10
	}
	if estimate > 500 {
		estimate = 500
	}

	fmt.Printf("Fetching emails from the last %s (up to %d)...\n", lastStr, estimate)

	msgs, err := client.ListRecentMessages(ctx, estimate)
	if err != nil {
		return fmt.Errorf("fetching messages: %w", err)
	}

	fmt.Printf("Found %d emails.\n", len(msgs))

	var proceed bool
	huh.NewConfirm().
		Title(fmt.Sprintf("Queue %d emails for labeling?", len(msgs))).
		Value(&proceed).
		Run()

	if !proceed {
		fmt.Println("Cancelled.")
		return nil
	}

	for _, m := range msgs {
		store.InsertMessage(m.ID, m.ThreadID)
	}

	fmt.Printf("Queued %d emails. They'll be processed by the daemon.\n", len(msgs))
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}
	switch unit {
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit %c (use d or h)", unit)
	}
}
```

**Step 4: Write config command (placeholder)**

```go
// internal/cli/config_cmd.go
package cli

import (
	"fmt"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "View or edit configuration",
		RunE:  runConfig,
	}
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Printf("Gmail:     %s\n", cfg.Gmail.Email)
	fmt.Printf("Provider:  %s\n", cfg.AI.Provider)
	fmt.Printf("Model:     %s\n", cfg.AI.Model)
	fmt.Printf("Base URL:  %s\n", cfg.AI.BaseURL)
	fmt.Printf("Poll:      %ds\n", cfg.PollInterval)
	fmt.Printf("\nLabels:\n")
	for _, l := range cfg.Labels {
		fmt.Printf("  - %s: %s\n", l.Name, l.Description)
	}
	fmt.Printf("\nConfig file: %s\n", config.DefaultPath())
	return nil
}
```

**Step 5: Write uninstall command**

```go
// internal/cli/uninstall.go
package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove background service and clean up",
		RunE:  runUninstall,
	}
}

func runUninstall(cmd *cobra.Command, args []string) error {
	mgr := service.Detect()
	if mgr != nil {
		fmt.Println("Removing background service...")
		if err := mgr.Uninstall(); err != nil {
			fmt.Printf("Warning: could not remove service: %v\n", err)
		}
	}

	var deleteData bool
	huh.NewConfirm().
		Title("Also delete all labelr data (~/.labelr/)?").
		Value(&deleteData).
		Run()

	if deleteData {
		if err := os.RemoveAll(config.Dir()); err != nil {
			return fmt.Errorf("removing data: %w", err)
		}
		fmt.Println("All labelr data deleted.")
	}

	fmt.Println("labelr uninstalled.")
	return nil
}
```

**Step 6: Register all commands in main.go**

Update main.go to add all remaining commands:

```go
rootCmd.AddCommand(
    cli.NewInitCmd(),
    cli.NewDaemonCmd(),
    cli.NewStartCmd(),
    cli.NewStopCmd(),
    cli.NewStatusCmd(),
    cli.NewLogsCmd(),
    cli.NewConfigCmd(),
    cli.NewSyncCmd(),
    cli.NewUninstallCmd(),
)
```

**Step 7: Verify build and run all tests**

Run: `make build && make test`
Expected: Build succeeds, all tests pass

**Step 8: Commit**

```bash
git add internal/cli/ cmd/labelr/main.go
git commit -m "feat: add status, logs, config, sync, and uninstall CLI commands"
```

---

### Task 17: Integration Testing

**Files:**
- Create: `internal/daemon/integration_test.go`

**Step 1: Write integration test for the full classify pipeline**

```go
// internal/daemon/integration_test.go
package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/gmail"
)

func TestFullPipeline(t *testing.T) {
	store, _ := db.Open(filepath.Join(t.TempDir(), "test.db"))
	defer store.Close()

	// Set up label mapping
	store.SetLabelMapping("Newsletter", "Label_NL")
	store.SetLabelMapping("Finance", "Label_FI")

	// Insert messages
	store.InsertMessage("msg1", "t1")
	store.InsertMessage("msg2", "t2")

	fetcher := &mockEmailFetcher{email: &gmail.EmailData{
		ID: "msg1", From: "news@example.com", Subject: "Weekly Digest",
	}}
	classifier := &mockClassifier{label: "Newsletter"}
	applier := &mockLabelApplier{}

	worker := NewWorker(store, fetcher, classifier, applier, nil)

	// Process first message
	processed, err := worker.ProcessOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("ProcessOne: processed=%v, err=%v", processed, err)
	}

	// Verify it was labeled
	msg, _ := store.GetMessage("msg1")
	if msg.Status != "labeled" {
		t.Errorf("msg1 status=%q, want labeled", msg.Status)
	}
	if !msg.Label.Valid || msg.Label.String != "Newsletter" {
		t.Errorf("msg1 label=%v, want Newsletter", msg.Label)
	}

	// Verify label was applied in Gmail
	if len(applier.applied) != 1 || applier.applied[0].LabelID != "Label_NL" {
		t.Errorf("unexpected applied labels: %v", applier.applied)
	}

	// Second message should still be pending
	msgs, _ := store.PendingMessages(10)
	if len(msgs) != 1 || msgs[0].ID != "msg2" {
		t.Error("expected msg2 still pending")
	}
}
```

**Step 2: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

**Step 3: Commit**

```bash
git add internal/daemon/integration_test.go
git commit -m "test: add integration test for full classify pipeline"
```

---

### Task 18: Final Wiring and Build

**Files:**
- Modify: `cmd/labelr/main.go` (final version)
- Modify: `Makefile` (add cross-compile targets)
- Create: `.gitignore`

**Step 1: Create .gitignore**

```
bin/
*.db
*.log
.labelr/
```

**Step 2: Update Makefile with cross-compile targets**

```makefile
.PHONY: build run test clean release

build:
	go build -o bin/labelr ./cmd/labelr

run:
	go run ./cmd/labelr

test:
	go test ./... -v

clean:
	rm -rf bin/

release:
	GOOS=darwin GOARCH=arm64 go build -o bin/labelr-darwin-arm64 ./cmd/labelr
	GOOS=darwin GOARCH=amd64 go build -o bin/labelr-darwin-amd64 ./cmd/labelr
	GOOS=linux GOARCH=amd64 go build -o bin/labelr-linux-amd64 ./cmd/labelr
	GOOS=windows GOARCH=amd64 go build -o bin/labelr-windows-amd64.exe ./cmd/labelr
```

**Step 3: Run full test suite and build**

Run: `make test && make build`
Expected: All tests pass, binary builds

**Step 4: Commit**

```bash
git add .gitignore Makefile cmd/labelr/main.go
git commit -m "chore: add gitignore, cross-compile targets, finalize main.go"
```

---

## Summary

| Task | Description | Key Files |
|------|-------------|-----------|
| 1 | Project scaffolding | go.mod, main.go, Makefile |
| 2 | Config package | internal/config/ |
| 3 | Database package | internal/db/ |
| 4 | Logger package | internal/log/ |
| 5 | Gmail auth | internal/gmail/auth.go |
| 6 | Gmail client | internal/gmail/client.go |
| 7 | AI providers | internal/ai/providers.go |
| 8 | AI classifier | internal/ai/classifier.go |
| 9 | Daemon poller | internal/daemon/poller.go |
| 10 | Daemon worker | internal/daemon/worker.go |
| 11 | Daemon orchestrator | internal/daemon/daemon.go |
| 12 | Service management | internal/service/ |
| 13 | CLI init command | internal/cli/init.go |
| 14 | CLI daemon command | internal/cli/daemon.go |
| 15 | CLI start/stop | internal/cli/start.go, stop.go |
| 16 | CLI remaining commands | internal/cli/status,logs,config,sync,uninstall |
| 17 | Integration testing | internal/daemon/integration_test.go |
| 18 | Final wiring + build | .gitignore, Makefile |
