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
