package cli

import (
	"fmt"
	"os"

	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/tui"
	"github.com/pankajbeniwal/labelr/internal/tui/logs"
	"github.com/spf13/cobra"
)

func NewLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Tail the daemon log file",
		RunE:  runLogs,
	}
}

func runLogs(cmd *cobra.Command, args []string) error {
	logPath := config.LogPath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s", logPath)
	}

	viewer := logs.NewViewer(logPath)
	return tui.Run(viewer)
}
