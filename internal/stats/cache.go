package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	SessionCacheVersion  = 1
	SessionCacheFilename = ".claude-stats-sessions.json"
)

type SourceFileRef struct {
	Path  string `json:"path"`
	Mtime int64  `json:"mtime"` // UnixNano
	Size  int64  `json:"size"`
}

type SessionCacheEntry struct {
	SessionInfo
	EncodedProject string          `json:"encodedProject"`
	Sources        []SourceFileRef `json:"sources"`
	LastIndexedAt  int64           `json:"lastIndexedAt"` // Unix seconds
}

type SessionCache struct {
	Version     int                           `json:"version"`
	GeneratedAt int64                         `json:"generatedAt"`
	Entries     map[string]*SessionCacheEntry `json:"entries"`
}

// SessionCachePath returns the absolute path of the cache file for the given
// claudeDir. An empty claudeDir falls back to ~/.claude.
func SessionCachePath(claudeDir string) string {
	if claudeDir == "" {
		home, _ := os.UserHomeDir()
		claudeDir = filepath.Join(home, ".claude")
	}
	return filepath.Join(claudeDir, SessionCacheFilename)
}

// LoadSessionCache reads the on-disk cache. A missing or corrupt file yields
// an empty cache and no error so the caller can transparently rebuild.
func LoadSessionCache(claudeDir string) (*SessionCache, error) {
	c := &SessionCache{
		Version: SessionCacheVersion,
		Entries: make(map[string]*SessionCacheEntry),
	}
	data, err := os.ReadFile(SessionCachePath(claudeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, nil
	}
	var disk SessionCache
	if err := json.Unmarshal(data, &disk); err != nil {
		return c, nil
	}
	if disk.Version != SessionCacheVersion || disk.Entries == nil {
		return c, nil
	}
	return &disk, nil
}

// SaveSessionCache writes the cache atomically (temp file + rename).
func SaveSessionCache(claudeDir string, c *SessionCache) error {
	c.Version = SessionCacheVersion
	c.GeneratedAt = time.Now().Unix()

	path := SessionCachePath(claudeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".claude-stats-sessions.*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// RefreshFull ignores any existing cache and rebuilds from every conversation
// file in the inventory. The resulting cache is written to disk before
// returning.
func RefreshFull(claudeDir string, inv *FileInventory) (*SessionCache, error) {
	cache := &SessionCache{
		Version: SessionCacheVersion,
		Entries: make(map[string]*SessionCacheEntry),
	}
	parsed := parseFilesParallel(inv.ConversationFiles)
	mergeParsed(cache, parsed)
	if err := SaveSessionCache(claudeDir, cache); err != nil {
		return cache, err
	}
	return cache, nil
}

// RefreshIncremental loads the existing cache and re-parses only the files
// whose mtime has advanced since they were indexed (plus any other file
// touching a session whose data needs recomputing). The updated cache is
// written back to disk.
func RefreshIncremental(claudeDir string, inv *FileInventory) (*SessionCache, error) {
	cache, err := LoadSessionCache(claudeDir)
	if err != nil {
		return nil, err
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]*SessionCacheEntry)
	}

	currentFiles := make(map[string]os.FileInfo, len(inv.ConversationFiles))
	for _, p := range inv.ConversationFiles {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		currentFiles[p] = info
	}

	// Reverse index: path -> set of session IDs that touch it.
	fileToSessions := make(map[string]map[string]struct{})
	// Cached mtime per path (latest seen across any session entry).
	cachedMtime := make(map[string]int64)
	for sid, e := range cache.Entries {
		for _, ref := range e.Sources {
			if fileToSessions[ref.Path] == nil {
				fileToSessions[ref.Path] = make(map[string]struct{})
			}
			fileToSessions[ref.Path][sid] = struct{}{}
			if ref.Mtime > cachedMtime[ref.Path] {
				cachedMtime[ref.Path] = ref.Mtime
			}
		}
	}

	// Stale = new file (no cache row) or mtime advanced.
	staleFiles := make(map[string]struct{})
	for path, info := range currentFiles {
		m, ok := cachedMtime[path]
		if !ok || info.ModTime().UnixNano() > m {
			staleFiles[path] = struct{}{}
		}
	}
	// Deleted = in cache but no longer on disk.
	deletedFiles := make(map[string]struct{})
	for path := range fileToSessions {
		if _, ok := currentFiles[path]; !ok {
			deletedFiles[path] = struct{}{}
		}
	}

	// Sessions to recompute = anyone touching a stale or deleted file.
	affected := make(map[string]struct{})
	for path := range staleFiles {
		for sid := range fileToSessions[path] {
			affected[sid] = struct{}{}
		}
	}
	for path := range deletedFiles {
		for sid := range fileToSessions[path] {
			affected[sid] = struct{}{}
		}
	}

	if len(affected) == 0 && len(staleFiles) == 0 && len(deletedFiles) == 0 {
		return cache, nil
	}

	// Drop affected entries; they will be rebuilt below.
	for sid := range affected {
		delete(cache.Entries, sid)
	}

	// Pass A: parse stale files.
	toParse := make([]string, 0, len(staleFiles))
	for p := range staleFiles {
		toParse = append(toParse, p)
	}
	parsedA := parseFilesParallel(toParse)

	// Collect every session ID that appeared in pass A — these may need
	// data from other (unchanged) files to be merged back in.
	touched := make(map[string]struct{})
	for _, pf := range parsedA {
		for sid := range pf.sessions {
			touched[sid] = struct{}{}
		}
	}
	// Include sessions that were affected but didn't appear in any stale file
	// (e.g. all their sources were deleted — nothing to do, but if some
	// sources remain we need to re-aggregate).
	for sid := range affected {
		touched[sid] = struct{}{}
	}

	// Pass B: for every current file not in pass A, check whether any of its
	// previously-known sessions are touched — if so, re-parse.
	var passB []string
	for path := range currentFiles {
		if _, done := staleFiles[path]; done {
			continue
		}
		for sid := range fileToSessions[path] {
			if _, ok := touched[sid]; ok {
				passB = append(passB, path)
				break
			}
		}
	}
	parsedB := parseFilesParallel(passB)

	mergeParsed(cache, parsedA)
	mergeParsed(cache, parsedB)

	if err := SaveSessionCache(claudeDir, cache); err != nil {
		return cache, err
	}
	return cache, nil
}

type parsedFile struct {
	path     string
	mtime    int64
	size     int64
	sessions map[string]*SessionInfo
}

// parseFilesParallel parses many .jsonl files concurrently using a small
// worker pool. Files that fail to parse are silently skipped (matching the
// existing scanner behaviour).
func parseFilesParallel(paths []string) map[string]parsedFile {
	out := make(map[string]parsedFile, len(paths))
	if len(paths) == 0 {
		return out
	}

	type item struct {
		path string
		pf   parsedFile
		ok   bool
	}
	results := make(chan item, len(paths))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for _, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()

			info, err := os.Stat(path)
			if err != nil {
				results <- item{path: path}
				return
			}
			sess, err := parseSessionsFromFile(path)
			if err != nil {
				results <- item{path: path}
				return
			}
			results <- item{
				path: path,
				pf: parsedFile{
					path:     path,
					mtime:    info.ModTime().UnixNano(),
					size:     info.Size(),
					sessions: sess,
				},
				ok: true,
			}
		}(p)
	}
	go func() { wg.Wait(); close(results) }()

	for r := range results {
		if r.ok {
			out[r.path] = r.pf
		}
	}
	return out
}

// mergeParsed folds parsed file results into the cache. For each session it
// sums message/tool counts, sums byte size, takes the earliest StartTime,
// the latest LastActivity, and keeps the last non-empty Title/Model/Version.
func mergeParsed(cache *SessionCache, parsed map[string]parsedFile) {
	now := time.Now().Unix()
	for path, pf := range parsed {
		ref := SourceFileRef{Path: path, Mtime: pf.mtime, Size: pf.size}
		for sid, partial := range pf.sessions {
			entry, ok := cache.Entries[sid]
			if !ok {
				entry = &SessionCacheEntry{
					SessionInfo:    *partial,
					EncodedProject: projectFromConvPath(path),
				}
				entry.Sources = []SourceFileRef{ref}
				entry.LastIndexedAt = now
				cache.Entries[sid] = entry
				continue
			}
			entry.MessageCount += partial.MessageCount
			entry.ToolCallCount += partial.ToolCallCount
			entry.SizeBytes += partial.SizeBytes
			if partial.StartTime != "" && (entry.StartTime == "" || partial.StartTime < entry.StartTime) {
				entry.StartTime = partial.StartTime
			}
			if partial.LastActivity != "" && partial.LastActivity > entry.LastActivity {
				entry.LastActivity = partial.LastActivity
			}
			if partial.Title != "" {
				entry.Title = partial.Title
			}
			if partial.Model != "" {
				entry.Model = partial.Model
			}
			if partial.Version != "" {
				entry.Version = partial.Version
			}
			if entry.Project == "" {
				entry.Project = partial.Project
			}
			if entry.EncodedProject == "" {
				entry.EncodedProject = projectFromConvPath(path)
			}
			entry.Sources = upsertSource(entry.Sources, ref)
			entry.LastIndexedAt = now
		}
	}
}

func upsertSource(refs []SourceFileRef, ref SourceFileRef) []SourceFileRef {
	for i, r := range refs {
		if r.Path == ref.Path {
			refs[i] = ref
			return refs
		}
	}
	return append(refs, ref)
}
