package cli

import (
	"github.com/Pankaj3112/labelr/internal/tui"
	"github.com/Pankaj3112/labelr/internal/tui/status"
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
	dashboard, err := status.New()
	if err != nil {
		return err
	}
	return tui.Run(dashboard)
}
