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
		Short: "Remove background service and clean up",
		RunE:  runUninstall,
	}
}

func runUninstall(cmd *cobra.Command, args []string) error {
	mgr := service.Detect()
	if mgr != nil {
		ui.Info("Removing background service...")
		if err := mgr.Uninstall(); err != nil {
			ui.Error(fmt.Sprintf("Could not remove service: %v", err))
		} else {
			ui.Success("Background service removed")
		}
	}

	var deleteData bool
	huh.NewConfirm().
		Title("Also delete all labelr data (~/.labelr/)?").
		Value(&deleteData).
		Run()

	if deleteData {
		if err := os.RemoveAll(config.Dir()); err != nil {
			return fmt.Errorf("removing data: %w", err)
		}
		ui.Success("All labelr data deleted")
	}

	ui.Success("labelr uninstalled")
	return nil
}
