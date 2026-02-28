package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Pankaj3112/labelr/internal/config"
	"github.com/Pankaj3112/labelr/internal/db"
	gmailpkg "github.com/Pankaj3112/labelr/internal/gmail"
	"github.com/Pankaj3112/labelr/internal/tui"
	tuisync "github.com/Pankaj3112/labelr/internal/tui/sync"
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

	view := tuisync.New(lastStr, duration, client, store)
	return tui.Run(view)
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
	case 'm':
		return time.Duration(num) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unknown unit %c (use d, h, or m)", unit)
	}
}
