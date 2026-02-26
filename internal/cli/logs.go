package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pankajbeniwal/labelr/internal/config"
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

	tailCmd := exec.Command("tail", "-f", logPath)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr
	return tailCmd.Run()
}
