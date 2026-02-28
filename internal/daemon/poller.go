package daemon

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Pankaj3112/labelr/internal/db"
	applog "github.com/Pankaj3112/labelr/internal/log"
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
		if err := p.store.SetState("history_id", strconv.FormatUint(newHistoryID, 10)); err != nil && p.logger != nil {
			p.logger.Error("saving history_id: %v", err)
		}
	}

	if p.logger != nil && len(messages) > 0 {
		p.logger.Info("polled %d new messages", len(messages))
	}

	return nil
}
