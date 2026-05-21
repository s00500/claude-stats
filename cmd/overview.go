package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var overviewCmd = &cobra.Command{
	Use:   "overview",
	Short: "High-level summary (default)",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()
		s, err := stats.ComputeOverview(inv, claudeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing overview: %v\n", err)
			os.Exit(1)
		}
		stats.PrintOverview(s, jsonOutput)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(overviewCmd)
	// Default behavior: running `claude-stats` with no subcommand runs overview.
	rootCmd.RunE = overviewCmd.RunE
}
