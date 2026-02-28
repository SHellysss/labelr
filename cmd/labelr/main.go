package main

import (
	"fmt"
	"os"

	"github.com/Pankaj3112/labelr/internal/cli"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "labelr",
		Short:   "AI-powered Gmail labeler",
		Long:    "labelr automatically classifies and labels your Gmail emails using AI.",
		Version: version,
	}

	rootCmd.AddCommand(
		cli.NewSetupCmd(),
		cli.NewDaemonCmd(),
		cli.NewStartCmd(),
		cli.NewStopCmd(),
		cli.NewStatusCmd(),
		cli.NewLogsCmd(),
		cli.NewSyncCmd(),
		cli.NewUninstallCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
