// Package main is the entry point for the oberwatch binary.
package main

import (
	"fmt"
	"os"

	"github.com/OberWatch/oberwatch/internal/config"
	"github.com/spf13/cobra"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	rootCmd := &cobra.Command{
		Use:   "oberwatch",
		Short: "Oberwatch — proxy and observability platform for AI agents",
	}

	rootCmd.AddCommand(config.NewInitCmd())
	rootCmd.AddCommand(config.NewValidateCmd())

	return rootCmd.Execute()
}
