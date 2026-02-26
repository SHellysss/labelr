package main

import (
	"fmt"
	"os"

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

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
