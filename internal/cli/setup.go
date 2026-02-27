package cli

import (
	"fmt"
	"os"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/tui"
	tuisetup "github.com/pankajbeniwal/labelr/internal/tui/setup"
	"github.com/spf13/cobra"
)

func NewSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up or reconfigure labelr",
		Long:  "First-time setup wizard or reconfigure existing settings: Gmail auth, AI provider, labels.",
		RunE:  runSetup,
	}
}

func runSetup(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Detect mode: first-run vs reconfigure
	existingCfg, cfgErr := config.Load(config.DefaultPath())
	if cfgErr == nil && existingCfg.AI.Provider != "" {
		view := tuisetup.NewReconfigureView(existingCfg)
		return tui.Run(view)
	}

	// First-time setup — launch TUI wizard
	wizard, err := tuisetup.NewWizard()
	if err != nil {
		return err
	}
	return tui.Run(wizard)
}
