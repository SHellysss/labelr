package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pankajbeniwal/labelr/internal/db"
)

// mockGmailPoller implements the GmailPoller interface for testing
type mockGmailPoller struct {
	messages     []struct{ ID, ThreadID string }
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
