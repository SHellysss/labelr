package cli

import (
	"fmt"
	"os"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the background service",
		RunE:  runStart,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(config.DefaultPath()); os.IsNotExist(err) {
		return fmt.Errorf("no config found — run 'labelr init' first")
	}

	mgr := service.Detect()
	if mgr == nil {
		return fmt.Errorf("unsupported operating system")
	}

	ui.Info("Starting labelr daemon...")
	if err := mgr.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	ui.Success("labelr is running in the background")
	ui.Dim("Use 'labelr status' to check on it")
	return nil
}
