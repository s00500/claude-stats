package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	config := parseFlags()
	initColors(config.NoColor)

	inv, err := DiscoverFiles(config.ClaudeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", config.ClaudeDir, err)
		os.Exit(1)
	}

	switch config.Command {
	case "", "overview":
		stats, err := ComputeOverview(inv, config.ClaudeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing overview: %v\n", err)
			os.Exit(1)
		}
		PrintOverview(stats, config.JSONOutput)

	case "sessions":
		sessions, err := ComputeSessionList(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing sessions: %v\n", err)
			os.Exit(1)
		}
		PrintSessions(sessions, config.JSONOutput)

	case "projects":
		projects, err := ComputeProjectStats(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing project stats: %v\n", err)
			os.Exit(1)
		}
		PrintProjects(projects, config.JSONOutput)

	case "secrets":
		if config.FilterProject != "" || config.FilterSession != "" {
			inv.ConversationFiles = FilterConversationFiles(inv.ConversationFiles, config.FilterProject, config.FilterSession)
		}
		fmt.Fprintf(os.Stderr, "Scanning %d conversation files for secrets...\n", len(inv.ConversationFiles))
		findings, err := ScanForSecrets(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning for secrets: %v\n", err)
			os.Exit(1)
		}
		PrintSecrets(findings, config.JSONOutput, config.RawSecrets)

		if config.Overwrite && len(findings) > 0 {
			fmt.Fprintf(os.Stderr, "\nOverwriting %d secrets across conversation files...\n", len(findings))
			files, censored, err := OverwriteSecrets(findings)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error overwriting secrets: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Done: censored %d occurrences in %d files.\n", censored, files)
		}

	case "storage":
		categories, err := ComputeStorageBreakdown(config.ClaudeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error computing storage: %v\n", err)
			os.Exit(1)
		}
		PrintStorage(categories, config.JSONOutput)

	case "memory":
		memories, err := ComputeMemoryList(inv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing memory: %v\n", err)
			os.Exit(1)
		}
		PrintMemory(memories, config.JSONOutput)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", config.Command)
		printUsage()
		os.Exit(1)
	}
}

func parseFlags() Config {
	var config Config

	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".claude")

	flag.StringVar(&config.ClaudeDir, "claude-dir", defaultDir, "Path to Claude data directory")
	flag.BoolVar(&config.JSONOutput, "json", false, "Output in JSON format")
	flag.BoolVar(&config.NoColor, "no-color", false, "Disable colored output")
	flag.BoolVar(&config.RawSecrets, "raw", false, "Show full unredacted secrets (secrets command)")
	flag.BoolVar(&config.Overwrite, "overwrite", false, "Censor secrets in actual conversation files (secrets command)")
	flag.StringVar(&config.FilterSession, "session", "", "Filter to a specific session ID (secrets command)")
	flag.StringVar(&config.FilterProject, "project", "", "Filter to projects matching substring (secrets command)")
	flag.Usage = printUsage
	flag.Parse()

	args := flag.Args()
	if len(args) > 0 {
		config.Command = args[0]
	}

	return config
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `claude-stats — analyze Claude Code local data

Usage: claude-stats [flags] [command]

Commands:
  overview    High-level summary (default)
  sessions    List sessions with stats
  projects    Per-project breakdown
  secrets     Scan for potential secrets/keys in conversations
  storage     Disk usage by category
  memory      List all memory storage locations

Flags:
  --claude-dir <path>   Path to Claude data directory (default: ~/.claude)
  --json                Output in JSON format
  --no-color            Disable colored output
  --raw                 Show full unredacted secrets (secrets command)
  --overwrite           Censor secrets in actual conversation files (secrets command)
  --project <substr>    Filter to projects matching substring (secrets command)
  --session <id>        Filter to a specific session ID (secrets command)
`)
}
