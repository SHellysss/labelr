package daemon

import (
	"context"
	"time"

	"github.com/pankajbeniwal/labelr/internal/db"
	applog "github.com/pankajbeniwal/labelr/internal/log"
)

type Daemon struct {
	store        *db.Store
	poller       *Poller
	worker       *Worker
	logger       *applog.Logger
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
