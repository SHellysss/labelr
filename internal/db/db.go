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
