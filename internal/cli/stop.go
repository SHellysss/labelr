package cli

import (
	"fmt"

	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the background service",
		RunE:  runStop,
	}
}

func runStop(cmd *cobra.Command, args []string) error {
	mgr := service.Detect()
	if mgr == nil {
		return fmt.Errorf("unsupported operating system")
	}

	fmt.Println("Stopping labelr daemon...")
	if err := mgr.Stop(); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}

	fmt.Println("labelr daemon stopped.")
	return nil
}
