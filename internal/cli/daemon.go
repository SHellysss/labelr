package cli

import (
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/daemon"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	applog "github.com/pankajbeniwal/labelr/internal/log"
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

func runDaemon(cmd *cobra.Command, args []string) error {
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

	// Create Gmail client
	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating Gmail token source: %w", err)
	}
	gmailClient, err := gmailpkg.NewClient(cmd.Context(), ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}

	// Create AI classifier
	apiKey := cfg.ResolveAPIKey()
	classifier := ai.NewClassifier(apiKey, cfg.AI.BaseURL, cfg.AI.Model, cfg.Labels)

	// Create daemon components
	poller := daemon.NewPoller(store, gmailClient, logger)
	worker := daemon.NewWorker(store, gmailClient, classifier, gmailClient, logger)

	d := daemon.New(store, poller, worker, logger, time.Duration(cfg.PollInterval)*time.Second)

	// Run with graceful shutdown
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return d.Run(ctx)
}
