package cli

import (
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/Pankaj3112/labelr/internal/ai"
	"github.com/Pankaj3112/labelr/internal/config"
	"github.com/Pankaj3112/labelr/internal/daemon"
	"github.com/Pankaj3112/labelr/internal/db"
	gmailpkg "github.com/Pankaj3112/labelr/internal/gmail"
	applog "github.com/Pankaj3112/labelr/internal/log"
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

const (
	initRetryBase = 5 * time.Second
	initRetryMax  = 2 * time.Minute
)

func runDaemon(cmd *cobra.Command, args []string) error {
	// Phase 1: Non-retryable init — fatal errors exit immediately.

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

	// Phase 2: Retryable init — transient errors (network, OAuth) are retried
	// with exponential backoff so the process stays alive and launchd's throttle
	// never triggers.

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var gmailClient *gmailpkg.Client
	backoff := initRetryBase
	for {
		ts, tsErr := gmailpkg.TokenSource(config.CredentialsPath())
		if tsErr == nil {
			gmailClient, err = gmailpkg.NewClient(ctx, ts)
			if err == nil {
				break
			}
			tsErr = err
		}

		log.Printf("daemon init failed (retrying in %s): %v", backoff, tsErr)

		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted during init retry: %w", ctx.Err())
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > initRetryMax {
			backoff = initRetryMax
		}
	}

	// Create AI classifier
	apiKey := cfg.ResolveAPIKey()
	classifier := ai.NewClassifier(apiKey, cfg.AI.BaseURL, cfg.AI.Model, cfg.Labels)

	// Create daemon components
	poller := daemon.NewPoller(store, gmailClient, logger)
	worker := daemon.NewWorker(store, gmailClient, classifier, gmailClient, logger)

	d := daemon.New(store, poller, worker, logger, time.Duration(cfg.PollInterval)*time.Second)

	return d.Run(ctx)
}
