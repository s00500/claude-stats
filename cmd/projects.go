package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Per-project breakdown",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()
		list, err := stats.ComputeProjectStats(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing project stats: %v\n", err)
			os.Exit(1)
		}
		stats.PrintProjects(list, jsonOutput)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(projectsCmd)
}
