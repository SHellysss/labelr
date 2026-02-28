package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Pankaj3112/labelr/internal/db"
	"github.com/Pankaj3112/labelr/internal/gmail"
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
