package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Fully uninstall labelr",
		RunE:  runUninstall,
	}
}

func runUninstall(cmd *cobra.Command, args []string) error {
	fmt.Println()

	// Stop and remove service
	mgr := service.Detect()
	if mgr != nil {
		ui.Info("Stopping daemon...")
		mgr.Stop()
		if err := mgr.Uninstall(); err != nil {
			ui.Error(fmt.Sprintf("Could not remove service: %v", err))
		} else {
			ui.Success("Background service removed")
		}
	}

	// Ask about data
	var keepData bool
	huh.NewConfirm().
		Title("Keep your data (~/.labelr/)?").
		Value(&keepData).
		Run()

	if keepData {
		ui.Info(fmt.Sprintf("Data kept at %s", config.Dir()))
	} else {
		if err := os.RemoveAll(config.Dir()); err != nil {
			return fmt.Errorf("removing data: %w", err)
		}
		ui.Success("All data deleted")
	}

	// Remove binary
	binaryPath, err := os.Executable()
	if err != nil {
		ui.Error("Could not determine binary path — remove manually")
	} else {
		if err := os.Remove(binaryPath); err != nil {
			ui.Error(fmt.Sprintf("Could not remove binary at %s: %v", binaryPath, err))
			ui.Dim("You may need to remove it manually or run with sudo")
		} else {
			ui.Success("Binary removed")
		}
	}

	fmt.Println()
	ui.Bold("labelr has been uninstalled.")
	fmt.Println()
	return nil
}
