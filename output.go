package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
)

var colorsEnabled = true

func initColors(noColor bool) {
	if noColor || os.Getenv("NO_COLOR") != "" {
		colorsEnabled = false
	}
}

func c(text, color string) string {
	if !colorsEnabled {
		return text
	}
	return color + text + colorReset
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func PrintOverview(stats *OverviewStats, jsonOut bool) {
	if jsonOut {
		printJSON(stats)
		return
	}

	fmt.Println(c("Claude Code Statistics", colorBold+colorCyan))
	fmt.Println(c(strings.Repeat("=", 50), colorDim))
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "  %s\t%s\n", c("Data Directory:", colorBold), stats.ClaudeDir)
	fmt.Fprintf(w, "  %s\t%s\n", c("Total Size:", colorBold), formatBytes(stats.TotalSizeBytes))
	if stats.DateRange[0] != "" {
		fmt.Fprintf(w, "  %s\t%s to %s\n", c("Date Range:", colorBold), stats.DateRange[0], stats.DateRange[1])
	}
	w.Flush()

	fmt.Println()
	fmt.Println(c("  Usage", colorBold+colorBlue))
	fmt.Println(c("  "+strings.Repeat("-", 40), colorDim))

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	suffix := ""
	if stats.StatsCacheUsed {
		suffix = c(" (from stats-cache through "+stats.StatsCacheThrough+")", colorDim)
	}
	fmt.Fprintf(w, "    Conversations:\t%s files (%s)\n", formatNumberInt(stats.ConversationCount), formatBytes(stats.ConvSizeBytes))
	fmt.Fprintf(w, "    Projects:\t%s\n", formatNumberInt(stats.ProjectCount))
	fmt.Fprintf(w, "    Sessions:\t%s%s\n", formatNumberInt(stats.SessionCount), suffix)
	fmt.Fprintf(w, "    Messages:\t%s%s\n", formatNumberInt(stats.MessageCount), suffix)
	fmt.Fprintf(w, "    Tool Calls:\t%s%s\n", formatNumberInt(stats.ToolCallCount), suffix)
	w.Flush()

	fmt.Println()
	fmt.Println(c("  Token Usage", colorBold+colorBlue))
	fmt.Println(c("  "+strings.Repeat("-", 40), colorDim))

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "    Input:\t%s\n", formatNumber(stats.Tokens.Input))
	fmt.Fprintf(w, "    Output:\t%s\n", formatNumber(stats.Tokens.Output))
	fmt.Fprintf(w, "    Cache Created:\t%s\n", formatNumber(stats.Tokens.CacheCreation))
	fmt.Fprintf(w, "    Cache Read:\t%s\n", formatNumber(stats.Tokens.CacheRead))
	totalTokens := stats.Tokens.Input + stats.Tokens.Output + stats.Tokens.CacheCreation + stats.Tokens.CacheRead
	fmt.Fprintf(w, "    %s\t%s\n", c("Total:", colorBold), c(formatNumber(totalTokens), colorBold))
	w.Flush()

	fmt.Println()
	fmt.Println(c("  Memory & Sessions", colorBold+colorBlue))
	fmt.Println(c("  "+strings.Repeat("-", 40), colorDim))

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "    Memory Files:\t%s (%s)\n", formatNumberInt(stats.MemoryFileCount), formatBytes(stats.MemorySizeBytes))
	fmt.Fprintf(w, "    Active Sessions:\t%s\n", formatNumberInt(stats.ActiveSessionCount))
	w.Flush()
	fmt.Println()
}

func PrintSessions(sessions []SessionInfo, jsonOut bool) {
	if jsonOut {
		printJSON(sessions)
		return
	}

	fmt.Println(c("Sessions", colorBold+colorCyan))
	fmt.Println(c(strings.Repeat("=", 50), colorDim))
	fmt.Printf("  Total: %s sessions\n\n", formatNumberInt(len(sessions)))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
		c("DATE", colorBold), c("MSGS", colorBold), c("TOOLS", colorBold),
		c("SIZE", colorBold), c("MODEL", colorBold), c("PROJECT", colorBold))
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
		"----", "----", "-----", "----", "-----", "-------")

	limit := len(sessions)
	if limit > 50 {
		limit = 50
	}
	for _, s := range sessions[:limit] {
		proj := s.Project
		if len(proj) > 50 {
			proj = "..." + proj[len(proj)-47:]
		}
		model := s.Model
		if len(model) > 25 {
			model = model[:25]
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
			s.StartTime, formatNumberInt(s.MessageCount), formatNumberInt(s.ToolCallCount),
			formatBytes(s.SizeBytes), model, proj)
	}
	w.Flush()

	if len(sessions) > 50 {
		fmt.Printf("\n  ... and %d more sessions (use --json for full list)\n", len(sessions)-50)
	}
	fmt.Println()
}

func PrintProjects(projects []ProjectStats, jsonOut bool) {
	if jsonOut {
		printJSON(projects)
		return
	}

	fmt.Println(c("Projects", colorBold+colorCyan))
	fmt.Println(c(strings.Repeat("=", 50), colorDim))
	fmt.Printf("  Total: %s projects\n\n", formatNumberInt(len(projects)))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
		c("SIZE", colorBold), c("CONVS", colorBold), c("MSGS", colorBold),
		c("MEM", colorBold), c("RANGE", colorBold), c("PROJECT", colorBold))
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
		"----", "-----", "----", "---", "-----", "-------")

	for _, p := range projects {
		proj := p.DisplayPath
		if len(proj) > 60 {
			proj = "..." + proj[len(proj)-57:]
		}
		dateRange := ""
		if p.DateRange[0] != "" {
			dateRange = p.DateRange[0][:7] + ".." + p.DateRange[1][:7]
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
			formatBytes(p.SizeBytes), formatNumberInt(p.ConversationCount),
			formatNumberInt(p.MessageCount), formatNumberInt(p.MemoryFileCount),
			dateRange, proj)
	}
	w.Flush()
	fmt.Println()
}

func PrintSecrets(findings []SecretFinding, jsonOut bool, raw bool) {
	if jsonOut {
		if raw {
			printJSON(findings)
		} else {
			cleaned := make([]SecretFinding, len(findings))
			copy(cleaned, findings)
			for i := range cleaned {
				cleaned[i].RawMatch = ""
			}
			printJSON(cleaned)
		}
		return
	}

	fmt.Println(c("Secret Scan Results", colorBold+colorCyan))
	fmt.Println(c(strings.Repeat("=", 50), colorDim))

	if len(findings) == 0 {
		fmt.Println(c("  No potential secrets detected.", colorGreen))
		fmt.Println()
		return
	}

	fmt.Printf("  %s potential secrets found\n\n", c(formatNumberInt(len(findings)), colorRed+colorBold))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
		c("PATTERN", colorBold), c("MATCH", colorBold), c("DATE", colorBold), c("PROJECT", colorBold))
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
		"-------", "-----", "----", "-------")

	for _, f := range findings {
		proj := f.Project
		if len(proj) > 40 {
			proj = "..." + proj[len(proj)-37:]
		}
		match := f.Match
		if raw {
			match = f.RawMatch
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
			c(f.PatternName, colorYellow), c(match, colorRed),
			f.Timestamp, proj)
	}
	w.Flush()
	fmt.Println()
	if raw {
		fmt.Println(c("  WARNING: Showing unredacted secrets!", colorRed+colorBold))
	} else {
		fmt.Println(c("  Note: Matches are redacted. Use --raw to show full values.", colorDim))
	}
	fmt.Println()
}

func PrintStorage(categories []StorageCategory, jsonOut bool) {
	if jsonOut {
		printJSON(categories)
		return
	}

	fmt.Println(c("Storage Breakdown", colorBold+colorCyan))
	fmt.Println(c(strings.Repeat("=", 50), colorDim))

	var total int64
	for _, cat := range categories {
		total += cat.SizeBytes
	}
	fmt.Printf("  Total: %s\n\n", c(formatBytes(total), colorBold))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
		c("CATEGORY", colorBold), c("SIZE", colorBold), c("PCT", colorBold),
		c("FILES", colorBold), c("PATH", colorBold))
	fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
		"--------", "----", "---", "-----", "----")

	for _, cat := range categories {
		pct := float64(0)
		if total > 0 {
			pct = float64(cat.SizeBytes) / float64(total) * 100
		}
		bar := strings.Repeat("█", int(pct/5))
		if pct > 0 && len(bar) == 0 {
			bar = "▏"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s %s\n",
			cat.Name, formatBytes(cat.SizeBytes),
			fmt.Sprintf("%5.1f%%", pct),
			c(bar, colorBlue))
		fmt.Fprintf(w, "  \t\t\t%s\t%s\n",
			formatNumberInt(cat.FileCount), cat.Path)
	}
	w.Flush()
	fmt.Println()
}

func PrintMemory(memories []MemoryInfo, jsonOut bool) {
	if jsonOut {
		printJSON(memories)
		return
	}

	fmt.Println(c("Memory Storage", colorBold+colorCyan))
	fmt.Println(c(strings.Repeat("=", 50), colorDim))
	fmt.Printf("  Total: %s memory files\n\n", formatNumberInt(len(memories)))

	currentProject := ""
	for _, m := range memories {
		if m.Project != currentProject {
			currentProject = m.Project
			proj := currentProject
			if len(proj) > 70 {
				proj = "..." + proj[len(proj)-67:]
			}
			fmt.Printf("  %s\n", c(proj, colorBold+colorBlue))
		}

		icon := "  "
		if m.IsIndex {
			icon = c("■", colorCyan)
		} else if m.MemType == "session" {
			icon = c("○", colorDim)
		} else {
			icon = c("●", colorGreen)
		}

		name := m.Name
		extra := ""
		if m.Description != "" {
			extra = c(" — "+m.Description, colorDim)
		}
		if m.MemType != "" && m.MemType != "index" && m.MemType != "session" {
			name += c(" ["+m.MemType+"]", colorYellow)
		}
		if m.SessionID != "" {
			name += c(" ("+m.SessionID[:8]+"...)", colorDim)
		}

		fmt.Printf("    %s %s (%s)%s\n", icon, name, formatBytes(m.SizeBytes), extra)
	}
	fmt.Println()
}
