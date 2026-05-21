package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List sessions with stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()
		list, err := stats.ComputeSessionList(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing sessions: %v\n", err)
			os.Exit(1)
		}
		stats.PrintSessions(list, jsonOutput)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
}
