package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Disk usage by category",
	RunE: func(cmd *cobra.Command, args []string) error {
		cats, err := stats.ComputeStorageBreakdown(claudeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing storage: %v\n", err)
			os.Exit(1)
		}
		stats.PrintStorage(cats, jsonOutput)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(storageCmd)
}
