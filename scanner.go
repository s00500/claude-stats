package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func DiscoverFiles(claudeDir string) (*FileInventory, error) {
	inv := &FileInventory{
		ProjectDirs: make(map[string]string),
	}

	for _, name := range []string{"history.jsonl", "history_1.jsonl"} {
		p := filepath.Join(claudeDir, name)
		if _, err := os.Stat(p); err == nil {
			inv.HistoryFiles = append(inv.HistoryFiles, p)
		}
	}

	scf := filepath.Join(claudeDir, "stats-cache.json")
	if _, err := os.Stat(scf); err == nil {
		inv.StatsCacheFile = scf
	}

	sessDir := filepath.Join(claudeDir, "sessions")
	if entries, err := os.ReadDir(sessDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				inv.SessionMetaFiles = append(inv.SessionMetaFiles, filepath.Join(sessDir, e.Name()))
			}
		}
	}

	projRoot := filepath.Join(claudeDir, "projects")
	if entries, err := os.ReadDir(projRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			projPath := filepath.Join(projRoot, e.Name())
			inv.ProjectDirs[e.Name()] = projPath

			subEntries, err := os.ReadDir(projPath)
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if se.IsDir() {
					if se.Name() == "memory" {
						scanMemoryDir(filepath.Join(projPath, "memory"), e.Name(), inv)
					} else {
						smPath := filepath.Join(projPath, se.Name(), "session-memory", "summary.md")
						if _, err := os.Stat(smPath); err == nil {
							inv.SessionMemFiles = append(inv.SessionMemFiles, smPath)
						}
						// subagent conversation files
						saDir := filepath.Join(projPath, se.Name(), "subagents")
						if saEntries, err := os.ReadDir(saDir); err == nil {
							for _, sae := range saEntries {
								if strings.HasSuffix(sae.Name(), ".jsonl") && !strings.HasSuffix(sae.Name(), ".wakatime") {
									inv.ConversationFiles = append(inv.ConversationFiles, filepath.Join(saDir, sae.Name()))
								}
							}
						}
					}
				} else if strings.HasSuffix(se.Name(), ".jsonl") && !strings.HasSuffix(se.Name(), ".wakatime") {
					inv.ConversationFiles = append(inv.ConversationFiles, filepath.Join(projPath, se.Name()))
				}
			}
		}
	}

	return inv, nil
}

func scanMemoryDir(dir string, _ string, inv *FileInventory) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.Name() == "MEMORY.md" {
			inv.MemoryIndexFiles = append(inv.MemoryIndexFiles, p)
		} else {
			inv.MemoryFiles = append(inv.MemoryFiles, p)
		}
	}
}

func ParseConversationFileStreaming(path string, callback func(ConversationLine) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var cl ConversationLine
		if err := json.Unmarshal(line, &cl); err != nil {
			continue
		}
		if err := callback(cl); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func ParseHistoryFile(path string) ([]HistoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

func ParseSessionMeta(path string) (*SessionMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sm SessionMeta
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, err
	}
	return &sm, nil
}

func ParseStatsCache(path string) (*StatsCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc StatsCache
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, err
	}
	return &sc, nil
}

func ParseMemoryFrontmatter(path string) (name, description, memType string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if inFrontmatter {
				break
			}
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			name = val
		case "description":
			description = val
		case "type":
			memType = val
		}
	}
	return name, description, memType, nil
}

func DecodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}
	if encoded[0] == '-' {
		encoded = encoded[1:]
	}
	parts := strings.Split(encoded, "-")
	if len(parts) < 2 {
		return "/" + encoded
	}

	// Greedy path reconstruction using os.Stat
	result := "/"
	i := 0
	for i < len(parts) {
		// try joining increasing numbers of parts as one path component
		bestLen := 0
		for j := i + 1; j <= len(parts); j++ {
			component := strings.Join(parts[i:j], "-")
			candidate := filepath.Join(result, component)
			if _, err := os.Stat(candidate); err == nil {
				bestLen = j - i
			}
		}
		if bestLen == 0 {
			bestLen = 1
		}
		component := strings.Join(parts[i:i+bestLen], "-")
		result = filepath.Join(result, component)
		i += bestLen
	}
	return result
}

func parseTimestamp(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}

	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		if n > 1e15 {
			return time.UnixMicro(n)
		}
		return time.UnixMilli(n)
	}

	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return time.UnixMilli(int64(f))
	}

	return time.Time{}
}

func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				parts = append(parts, b.Text)
			case "thinking":
				parts = append(parts, b.Thinking)
			case "tool_use":
				if len(b.Input) > 0 {
					parts = append(parts, string(b.Input))
				}
			case "tool_result":
				if len(b.Content) > 0 {
					parts = append(parts, string(b.Content))
				}
			}
		}
		return strings.Join(parts, "\n")
	}

	return string(raw)
}

func DirSize(path string) (int64, int) {
	var totalBytes int64
	var fileCount int
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalBytes += info.Size()
			fileCount++
		}
		return nil
	})
	return totalBytes, fileCount
}

func FileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func formatTimestampMillis(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04")
}

func timestampToDateStr(raw json.RawMessage) string {
	t := parseTimestamp(raw)
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func updateDateRange(r *[2]string, dateStr string) {
	if dateStr == "" {
		return
	}
	if r[0] == "" || dateStr < r[0] {
		r[0] = dateStr
	}
	if r[1] == "" || dateStr > r[1] {
		r[1] = dateStr
	}
}

func projectFromConvPath(convPath string) string {
	// .claude/projects/<encoded-name>/<uuid>.jsonl
	dir := filepath.Dir(convPath)
	base := filepath.Base(dir)
	// handle subagent paths: projects/<name>/<uuid>/subagents/<file>.jsonl
	if base == "subagents" {
		dir = filepath.Dir(filepath.Dir(dir))
		base = filepath.Base(dir)
	}
	return base
}

func countFromStatsCache(sc *StatsCache) (messages, sessions, tools int) {
	for _, d := range sc.DailyActivity {
		messages += d.MessageCount
		sessions += d.SessionCount
		tools += d.ToolCallCount
	}
	return
}

func formatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	remainder := len(s) % 3
	if remainder > 0 {
		result = append(result, s[:remainder]...)
	}
	for i := remainder; i < len(s); i += 3 {
		if len(result) > 0 {
			result = append(result, ',')
		}
		result = append(result, s[i:i+3]...)
	}
	return string(result)
}

func formatNumberInt(n int) string {
	return formatNumber(int64(n))
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func FilterConversationFiles(files []string, projectFilter, sessionFilter string) []string {
	if projectFilter == "" && sessionFilter == "" {
		return files
	}

	var result []string

	projectFilter = strings.ToLower(projectFilter)

	for _, f := range files {
		if projectFilter != "" {
			proj := projectFromConvPath(f)
			decoded := strings.ToLower(DecodeProjectPath(proj))
			encoded := strings.ToLower(proj)
			if !strings.Contains(decoded, projectFilter) && !strings.Contains(encoded, projectFilter) {
				continue
			}
		}

		if sessionFilter != "" {
			matchesSession := false
			_ = ParseConversationFileStreaming(f, func(cl ConversationLine) error {
				if cl.SessionID == sessionFilter {
					matchesSession = true
					return fmt.Errorf("found")
				}
				return nil
			})
			if !matchesSession {
				continue
			}
		}

		result = append(result, f)
	}
	return result
}
