package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"claude-stats/internal/stats"
)

var (
	secretsRaw       bool
	secretsOverwrite bool
	secretsProject   string
	secretsSession   string
)

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Scan for potential secrets/keys in conversations",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv := discoverOrExit()

		if secretsProject != "" || secretsSession != "" {
			inv.ConversationFiles = stats.FilterConversationFiles(inv.ConversationFiles, secretsProject, secretsSession)
		}

		fmt.Fprintf(os.Stderr, "Scanning %d conversation files for secrets...\n", len(inv.ConversationFiles))
		findings, err := stats.ScanForSecrets(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning for secrets: %v\n", err)
			os.Exit(1)
		}
		stats.PrintSecrets(findings, jsonOutput, secretsRaw)

		if secretsOverwrite && len(findings) > 0 {
			fmt.Fprintf(os.Stderr, "\nOverwriting %d secrets across conversation files...\n", len(findings))
			files, censored, err := stats.OverwriteSecrets(findings)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error overwriting secrets: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Done: censored %d occurrences in %d files.\n", censored, files)
		}
		return nil
	},
}

func init() {
	secretsCmd.Flags().BoolVar(&secretsRaw, "raw", false, "Show full unredacted secrets")
	secretsCmd.Flags().BoolVar(&secretsOverwrite, "overwrite", false, "Censor secrets in actual conversation files")
	secretsCmd.Flags().StringVar(&secretsProject, "project", "", "Filter to projects matching substring")
	secretsCmd.Flags().StringVar(&secretsSession, "session", "", "Filter to a specific session ID")
	rootCmd.AddCommand(secretsCmd)
}
