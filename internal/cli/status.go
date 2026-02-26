package cli

import (
	"fmt"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status and queue stats",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Check running status
	mgr := service.Detect()
	running := false
	if mgr != nil {
		running, _ = mgr.IsRunning()
	}

	if running {
		fmt.Println("Status:    running")
	} else {
		fmt.Println("Status:    stopped")
	}

	// Load config for provider info
	cfg, err := config.Load(config.DefaultPath())
	if err == nil {
		fmt.Printf("Provider:  %s / %s\n", cfg.AI.Provider, cfg.AI.Model)
		fmt.Printf("Account:   %s\n", cfg.Gmail.Email)
	}

	// Queue stats
	store, err := db.Open(config.DBPath())
	if err == nil {
		defer store.Close()
		stats, err := store.Stats()
		if err == nil {
			fmt.Printf("\nQueue:\n")
			fmt.Printf("  Pending:  %d\n", stats.Pending)
			fmt.Printf("  Labeled:  %d\n", stats.Labeled)
			fmt.Printf("  Failed:   %d\n", stats.Failed)
		}

		if lastPoll, err := store.GetState("last_poll_time"); err == nil {
			fmt.Printf("\nLast poll: %s\n", lastPoll)
		}
	}

	return nil
}
