package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"claude-stats/internal/stats"
	"claude-stats/internal/tui"
)

var sessionsNoTUI bool

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Browse or list sessions",
	Long: `Interactively browse sessions in a TUI. Use --no-tui or --json to fall
back to the plain table; the plain table is also used automatically when
stdout is not a terminal.

In the TUI, type '/' to filter, press 'd' / 'n' / 'a' / 'p' to sort by date,
name, age or project, 'r' to reverse the order, Enter to print the highlighted
session's details, and 'q' or Esc to quit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()
		list, err := stats.ComputeSessionList(claudeDir, inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing sessions: %v\n", err)
			os.Exit(1)
		}

		if jsonOutput {
			stats.PrintSessions(list, true)
			return nil
		}
		if sessionsNoTUI || !term.IsTerminal(int(os.Stdout.Fd())) {
			stats.PrintSessions(list, false)
			return nil
		}

		sel, err := tui.Run(list)
		if err != nil {
			return err
		}
		if sel != nil {
			fmt.Print(tui.RenderDetail(*sel))
		}
		return nil
	},
}

func init() {
	sessionsCmd.Flags().BoolVar(&sessionsNoTUI, "no-tui", false, "Print the plain table instead of launching the interactive browser")
	rootCmd.AddCommand(sessionsCmd)
}
