package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Install and start background service",
		RunE:  runStart,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check config exists
	if _, err := os.Stat(config.DefaultPath()); os.IsNotExist(err) {
		return fmt.Errorf("no config found. Run 'labelr init' first")
	}

	// Find our binary path
	binaryPath, err := exec.LookPath("labelr")
	if err != nil {
		// Fall back to current executable
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("finding labelr binary: %w", err)
		}
	}

	mgr := service.Detect()
	if mgr == nil {
		return fmt.Errorf("unsupported operating system")
	}

	// Install the service
	fmt.Println("Installing background service...")
	if err := mgr.Install(binaryPath); err != nil {
		return fmt.Errorf("installing service: %w", err)
	}

	// Start it
	fmt.Println("Starting labelr daemon...")
	if err := mgr.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	fmt.Println("labelr is now running in the background.")
	fmt.Println("Use 'labelr status' to check on it, or 'labelr stop' to stop it.")
	return nil
}
