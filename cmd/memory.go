package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "List all memory storage locations",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()
		list, err := stats.ComputeMemoryList(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing memory: %v\n", err)
			os.Exit(1)
		}
		stats.PrintMemory(list, jsonOutput)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(memoryCmd)
}
