package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

type ActivityEntry struct {
	Subject     string
	Label       string
	Status      string // "labeled" or "failed"
	ProcessedAt string
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

	// Add color columns (safe to run on existing DBs — ignores "duplicate column" errors)
	if err := s.migrateAddColumn("label_mappings", "bg_color", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("adding bg_color column: %w", err)
	}
	if err := s.migrateAddColumn("label_mappings", "text_color", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("adding text_color column: %w", err)
	}

	// Add subject column to messages for activity feed
	if err := s.migrateAddColumn("messages", "subject", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("adding subject column: %w", err)
	}

	return nil
}

func (s *Store) migrateAddColumn(table, column, colDef string) error {
	_, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef))
	if err != nil {
		// Ignore "duplicate column" errors (column already exists)
		if strings.Contains(err.Error(), "duplicate column") {
			return nil
		}
		return err
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

// MessageExists returns true if a message with the given ID is already in the DB.
func (s *Store) MessageExists(id string) bool {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE id = ?`, id).Scan(&count)
	return err == nil && count > 0
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

func (s *Store) MarkLabeled(id, label, subject string) error {
	_, err := s.db.Exec(
		`UPDATE messages SET status = 'labeled', label = ?, subject = ?, processed_at = datetime('now') WHERE id = ?`,
		label, subject, id,
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

// RecentActivity returns the most recently processed messages (labeled or failed).
func (s *Store) RecentActivity(limit int) ([]ActivityEntry, error) {
	rows, err := s.db.Query(
		`SELECT subject, COALESCE(label, ''), status, processed_at
		 FROM messages
		 WHERE status IN ('labeled', 'failed') AND processed_at IS NOT NULL
		 ORDER BY processed_at DESC
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.Subject, &e.Label, &e.Status, &e.ProcessedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
