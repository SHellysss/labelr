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
	store.MarkLabeled("msg1", "Finance", "Invoice #123")

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

func TestStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := Open(path)
	defer store.Close()

	store.InsertMessage("msg1", "t1")
	store.InsertMessage("msg2", "t2")
	store.InsertMessage("msg3", "t3")
	store.MarkProcessing("msg2")
	store.MarkLabeled("msg2", "Finance", "Budget report")

	stats, _ := store.Stats()
	if stats.Pending != 2 {
		t.Errorf("pending=%d, want 2", stats.Pending)
	}
	if stats.Labeled != 1 {
		t.Errorf("labeled=%d, want 1", stats.Labeled)
	}
}
