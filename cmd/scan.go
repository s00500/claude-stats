package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Force-rebuild the sessions cache",
	Long: `Scan walks every conversation file under the Claude data directory and
rebuilds the session cache from scratch. Subsequent 'sessions' invocations
will use the cache and only re-parse files whose mtime has advanced.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()
		c, err := stats.RefreshFull(claudeDir, inv)
		if err != nil {
			return err
		}
		fmt.Printf("Indexed %d sessions across %d files → %s\n",
			len(c.Entries), len(inv.ConversationFiles), stats.SessionCachePath(claudeDir))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
