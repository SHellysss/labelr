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
