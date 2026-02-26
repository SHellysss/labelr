package cli

import (
	"fmt"
	"time"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/ui"
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
	mgr := service.Detect()
	running := false
	if mgr != nil {
		running, _ = mgr.IsRunning()
	}

	fmt.Println()

	// Status line
	cfg, err := config.Load(config.DefaultPath())
	if err == nil {
		statusText := "stopped"
		if running {
			statusText = "running"
		}
		fmt.Printf("  %s %-12s %s / %s\n", ui.StatusDot(running), statusText, cfg.AI.Provider, cfg.AI.Model)
		fmt.Printf("  ✉ %s\n", cfg.Gmail.Email)
	} else {
		statusText := "stopped"
		if running {
			statusText = "running"
		}
		fmt.Printf("  %s %s\n", ui.StatusDot(running), statusText)
	}

	// Queue stats
	store, err := db.Open(config.DBPath())
	if err == nil {
		defer store.Close()
		stats, err := store.Stats()
		if err == nil {
			fmt.Printf("\n  Queue\n")
			fmt.Println("  ──────────────────────")
			fmt.Printf("  Pending   %s\n", ui.Yellow(fmt.Sprintf("%d", stats.Pending)))
			fmt.Printf("  Labeled   %s\n", ui.Green(fmt.Sprintf("%d", stats.Labeled)))
			fmt.Printf("  Failed    %s\n", ui.Red(fmt.Sprintf("%d", stats.Failed)))
		}

		if lastPoll, err := store.GetState("last_poll_time"); err == nil {
			if t, err := time.Parse(time.RFC3339, lastPoll); err == nil {
				fmt.Printf("\n  Last poll: %s\n", relativeTime(t))
			} else {
				fmt.Printf("\n  Last poll: %s\n", lastPoll)
			}
		}
	}

	fmt.Println()
	return nil
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
