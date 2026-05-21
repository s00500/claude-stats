package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var (
	claudeDir  string
	jsonOutput bool
	noColor    bool
)

var rootCmd = &cobra.Command{
	Use:   "claude-stats",
	Short: "Analyze Claude Code local data",
	Long: `claude-stats inspects ~/.claude to report on conversations, sessions,
projects, memory, secrets, and storage.

Running claude-stats with no subcommand prints the overview.`,
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		stats.InitColors(noColor)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".claude")

	rootCmd.PersistentFlags().StringVar(&claudeDir, "claude-dir", defaultDir, "Path to Claude data directory")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
}

func discoverOrExit() *stats.FileInventory {
	inv, err := stats.DiscoverFiles(claudeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", claudeDir, err)
		os.Exit(1)
	}
	return inv
}
