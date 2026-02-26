package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/spf13/cobra"
)

func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "One-time backlog scan",
		Long:  "Fetch and queue recent emails for labeling. Example: labelr sync --last 7d",
		RunE:  runSync,
	}
	cmd.Flags().String("last", "7d", "How far back to sync (e.g., 1d, 7d, 30d)")
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	lastStr, _ := cmd.Flags().GetString("last")
	duration, err := parseDuration(lastStr)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", lastStr, err)
	}

	_, err = config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating token source: %w", err)
	}

	ctx := context.Background()
	client, err := gmailpkg.NewClient(ctx, ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}

	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Estimate: ~50 emails per day
	estimate := int64(duration.Hours()/24) * 50
	if estimate < 10 {
		estimate = 10
	}
	if estimate > 500 {
		estimate = 500
	}

	fmt.Printf("Fetching emails from the last %s (up to %d)...\n", lastStr, estimate)

	msgs, err := client.ListRecentMessages(ctx, estimate)
	if err != nil {
		return fmt.Errorf("fetching messages: %w", err)
	}

	fmt.Printf("Found %d emails.\n", len(msgs))

	var proceed bool
	huh.NewConfirm().
		Title(fmt.Sprintf("Queue %d emails for labeling?", len(msgs))).
		Value(&proceed).
		Run()

	if !proceed {
		fmt.Println("Cancelled.")
		return nil
	}

	for _, m := range msgs {
		store.InsertMessage(m.ID, m.ThreadID)
	}

	fmt.Printf("Queued %d emails. They'll be processed by the daemon.\n", len(msgs))
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, err
	}
	switch unit {
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit %c (use d or h)", unit)
	}
}
