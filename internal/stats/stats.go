package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

func ComputeOverview(inv *FileInventory, claudeDir string) (*OverviewStats, error) {
	stats := &OverviewStats{
		ClaudeDir:     claudeDir,
		ProjectCount:  len(inv.ProjectDirs),
		MemoryFileCount: len(inv.MemoryFiles) + len(inv.MemoryIndexFiles),
	}

	totalSize, _ := DirSize(claudeDir)
	stats.TotalSizeBytes = totalSize

	for _, f := range inv.MemoryFiles {
		stats.MemorySizeBytes += FileSize(f)
	}
	for _, f := range inv.MemoryIndexFiles {
		stats.MemorySizeBytes += FileSize(f)
	}

	stats.ConversationCount = len(inv.ConversationFiles)
	for _, f := range inv.ConversationFiles {
		stats.ConvSizeBytes += FileSize(f)
	}

	for _, f := range inv.SessionMetaFiles {
		sm, err := ParseSessionMeta(f)
		if err != nil {
			continue
		}
		if sm.Status == "running" || sm.Status == "active" {
			stats.ActiveSessionCount++
		}
	}

	if inv.StatsCacheFile != "" {
		sc, err := ParseStatsCache(inv.StatsCacheFile)
		if err == nil && len(sc.DailyActivity) > 0 {
			stats.StatsCacheUsed = true
			stats.StatsCacheThrough = sc.LastComputedDate
			msgs, sess, tools := countFromStatsCache(sc)
			stats.MessageCount = msgs
			stats.SessionCount = sess
			stats.ToolCallCount = tools
			stats.DateRange[0] = sc.DailyActivity[0].Date
			stats.DateRange[1] = sc.DailyActivity[len(sc.DailyActivity)-1].Date
		}
	}

	// Scan all conversation files for token usage and to supplement counts
	// beyond what stats-cache covers
	type fileStat struct {
		messages  int
		tools     int
		tokens    TokenTotals
		dateRange [2]string
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, convFile := range inv.ConversationFiles {
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()

			var fs fileStat
			seenRequests := make(map[string]bool)

			_ = ParseConversationFileStreaming(path, func(cl ConversationLine) error {
				dateStr := timestampToDateStr(cl.Timestamp)
				updateDateRange(&fs.dateRange, dateStr)

				switch cl.Type {
				case "user":
					fs.messages++
				case "assistant":
					fs.messages++
					if cl.Message != nil && cl.Message.Content != nil {
						var blocks []ContentBlock
						if err := safeUnmarshalBlocks(cl.Message.Content, &blocks); err == nil {
							for _, b := range blocks {
								if b.Type == "tool_use" {
									fs.tools++
								}
							}
						}
					}
					if cl.Message != nil && cl.Message.Usage != nil {
						reqID := cl.RequestID
						if reqID == "" {
							reqID = cl.UUID
						}
						if !seenRequests[reqID] {
							seenRequests[reqID] = true
							fs.tokens.Input += int64(cl.Message.Usage.InputTokens)
							fs.tokens.Output += int64(cl.Message.Usage.OutputTokens)
							fs.tokens.CacheCreation += int64(cl.Message.Usage.CacheCreationInputTokens)
							fs.tokens.CacheRead += int64(cl.Message.Usage.CacheReadInputTokens)
						}
					}
				}
				return nil
			})

			mu.Lock()
			stats.Tokens.Input += fs.tokens.Input
			stats.Tokens.Output += fs.tokens.Output
			stats.Tokens.CacheCreation += fs.tokens.CacheCreation
			stats.Tokens.CacheRead += fs.tokens.CacheRead
			updateDateRange(&stats.DateRange, fs.dateRange[0])
			updateDateRange(&stats.DateRange, fs.dateRange[1])

			if !stats.StatsCacheUsed {
				stats.MessageCount += fs.messages
				stats.ToolCallCount += fs.tools
			}
			mu.Unlock()
		}(convFile)
	}
	wg.Wait()

	if !stats.StatsCacheUsed {
		stats.SessionCount = countUniqueSessions(inv)
	}

	return stats, nil
}

func countUniqueSessions(inv *FileInventory) int {
	seen := make(map[string]bool)
	for _, f := range inv.ConversationFiles {
		_ = ParseConversationFileStreaming(f, func(cl ConversationLine) error {
			if cl.SessionID != "" {
				seen[cl.SessionID] = true
			}
			return nil
		})
	}
	for _, f := range inv.HistoryFiles {
		entries, err := ParseHistoryFile(f)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.SessionID != "" {
				seen[e.SessionID] = true
			}
		}
	}
	return len(seen)
}

func ComputeProjectStats(inv *FileInventory) ([]ProjectStats, error) {
	projects := make(map[string]*ProjectStats)

	for name := range inv.ProjectDirs {
		projects[name] = &ProjectStats{
			EncodedName: name,
			DisplayPath: DecodeProjectPath(name),
		}
	}

	for _, convFile := range inv.ConversationFiles {
		proj := projectFromConvPath(convFile)
		ps, ok := projects[proj]
		if !ok {
			ps = &ProjectStats{EncodedName: proj, DisplayPath: DecodeProjectPath(proj)}
			projects[proj] = ps
		}
		ps.ConversationCount++
		ps.SizeBytes += FileSize(convFile)

		_ = ParseConversationFileStreaming(convFile, func(cl ConversationLine) error {
			dateStr := timestampToDateStr(cl.Timestamp)
			updateDateRange(&ps.DateRange, dateStr)
			switch cl.Type {
			case "user":
				ps.MessageCount++
			case "assistant":
				ps.MessageCount++
				if cl.Message != nil && cl.Message.Content != nil {
					var blocks []ContentBlock
					if err := safeUnmarshalBlocks(cl.Message.Content, &blocks); err == nil {
						for _, b := range blocks {
							if b.Type == "tool_use" {
								ps.ToolCallCount++
							}
						}
					}
				}
			}
			return nil
		})
	}

	for _, memFile := range inv.MemoryFiles {
		proj := projectFromMemoryPath(memFile)
		if ps, ok := projects[proj]; ok {
			ps.MemoryFileCount++
		}
	}
	for _, memFile := range inv.MemoryIndexFiles {
		proj := projectFromMemoryPath(memFile)
		if ps, ok := projects[proj]; ok {
			ps.MemoryFileCount++
		}
	}

	var result []ProjectStats
	for _, ps := range projects {
		if ps.ConversationCount > 0 || ps.MemoryFileCount > 0 {
			result = append(result, *ps)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SizeBytes > result[j].SizeBytes
	})
	return result, nil
}

func projectFromMemoryPath(memPath string) string {
	// .claude/projects/<encoded-name>/memory/file.md
	dir := filepath.Dir(memPath) // memory/
	dir = filepath.Dir(dir)      // <encoded-name>/
	return filepath.Base(dir)
}

func ComputeSessionList(inv *FileInventory) ([]SessionInfo, error) {
	type sessionAccum struct {
		SessionInfo
		sessions map[string]bool
	}
	sessions := make(map[string]*sessionAccum)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, convFile := range inv.ConversationFiles {
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()

			proj := projectFromConvPath(path)
			fsize := FileSize(path)
			local := make(map[string]*sessionAccum)

			_ = ParseConversationFileStreaming(path, func(cl ConversationLine) error {
				sid := cl.SessionID
				if sid == "" {
					return nil
				}
				sa, ok := local[sid]
				if !ok {
					sa = &sessionAccum{}
					sa.SessionID = sid
					sa.Project = DecodeProjectPath(proj)
					local[sid] = sa
				}

				dateStr := timestampToDateStr(cl.Timestamp)
				if sa.StartTime == "" || (dateStr != "" && dateStr < sa.StartTime) {
					t := parseTimestamp(cl.Timestamp)
					if !t.IsZero() {
						sa.StartTime = t.Format("2006-01-02 15:04")
					}
				}

				if cl.Version != "" {
					sa.Version = cl.Version
				}

				switch cl.Type {
				case "user":
					sa.MessageCount++
				case "assistant":
					sa.MessageCount++
					if cl.Message != nil {
						if cl.Message.Model != "" {
							sa.Model = cl.Message.Model
						}
						if cl.Message.Content != nil {
							var blocks []ContentBlock
							if err := safeUnmarshalBlocks(cl.Message.Content, &blocks); err == nil {
								for _, b := range blocks {
									if b.Type == "tool_use" {
										sa.ToolCallCount++
									}
								}
							}
						}
					}
				case "ai-title":
					if cl.Slug != "" {
						sa.Title = cl.Slug
					}
				}
				return nil
			})

			mu.Lock()
			for sid, sa := range local {
				if existing, ok := sessions[sid]; ok {
					existing.MessageCount += sa.MessageCount
					existing.ToolCallCount += sa.ToolCallCount
					existing.SizeBytes += fsize
					if sa.Title != "" {
						existing.Title = sa.Title
					}
					if sa.Model != "" {
						existing.Model = sa.Model
					}
				} else {
					sa.SizeBytes = fsize
					sessions[sid] = sa
				}
			}
			mu.Unlock()
		}(convFile)
	}
	wg.Wait()

	var result []SessionInfo
	for _, sa := range sessions {
		result = append(result, sa.SessionInfo)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime > result[j].StartTime
	})
	return result, nil
}

func ComputeStorageBreakdown(claudeDir string) ([]StorageCategory, error) {
	categories := []struct {
		name string
		path string
	}{
		{"Conversations", "projects"},
		{"Telemetry", "telemetry"},
		{"File History", "file-history"},
		{"Plugins", "plugins"},
		{"Todos", "todos"},
		{"Plans", "plans"},
		{"Shell Snapshots", "shell-snapshots"},
		{"Backups", "backups"},
		{"Paste Cache", "paste-cache"},
		{"Cache", "cache"},
		{"Tasks", "tasks"},
		{"Sessions", "sessions"},
		{"IDE", "ide"},
		{"Logs", "logs"},
	}

	var result []StorageCategory
	var accountedFor int64

	for _, cat := range categories {
		p := filepath.Join(claudeDir, cat.path)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		size, count := DirSize(p)
		accountedFor += size
		result = append(result, StorageCategory{
			Name:      cat.name,
			Path:      cat.path,
			SizeBytes: size,
			FileCount: count,
		})
	}

	// top-level files
	var topFileSize int64
	var topFileCount int
	entries, _ := os.ReadDir(claudeDir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		topFileSize += info.Size()
		topFileCount++
	}
	accountedFor += topFileSize
	result = append(result, StorageCategory{
		Name:      "Config & History",
		Path:      "(top-level files)",
		SizeBytes: topFileSize,
		FileCount: topFileCount,
	})

	totalSize, _ := DirSize(claudeDir)
	other := totalSize - accountedFor
	if other > 0 {
		result = append(result, StorageCategory{
			Name:      "Other",
			Path:      "(uncategorized)",
			SizeBytes: other,
			FileCount: 0,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SizeBytes > result[j].SizeBytes
	})
	return result, nil
}

func ComputeMemoryList(inv *FileInventory) ([]MemoryInfo, error) {
	var result []MemoryInfo

	for _, f := range inv.MemoryIndexFiles {
		proj := projectFromMemoryPath(f)
		result = append(result, MemoryInfo{
			Project:   DecodeProjectPath(proj),
			FilePath:  f,
			Name:      "MEMORY.md",
			MemType:   "index",
			SizeBytes: FileSize(f),
			IsIndex:   true,
		})
	}

	for _, f := range inv.MemoryFiles {
		proj := projectFromMemoryPath(f)
		name, desc, mtype, _ := ParseMemoryFrontmatter(f)
		if name == "" {
			name = filepath.Base(f)
		}
		result = append(result, MemoryInfo{
			Project:     DecodeProjectPath(proj),
			FilePath:    f,
			Name:        name,
			Description: desc,
			MemType:     mtype,
			SizeBytes:   FileSize(f),
		})
	}

	for _, f := range inv.SessionMemFiles {
		parts := strings.Split(f, string(os.PathSeparator))
		var proj, sessID string
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				proj = parts[i+1]
			}
			if p == "session-memory" && i-1 >= 0 {
				sessID = parts[i-1]
			}
		}
		result = append(result, MemoryInfo{
			Project:   DecodeProjectPath(proj),
			FilePath:  f,
			Name:      "Session Summary",
			MemType:   "session",
			SizeBytes: FileSize(f),
			SessionID: sessID,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Project < result[j].Project
	})
	return result, nil
}

func safeUnmarshalBlocks(raw []byte, blocks *[]ContentBlock) error {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	return json.Unmarshal(raw, blocks)
}
