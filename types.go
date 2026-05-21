package main

import "encoding/json"

type Config struct {
	ClaudeDir     string
	JSONOutput    bool
	NoColor       bool
	RawSecrets    bool
	Overwrite     bool
	FilterSession string
	FilterProject string
	Command       string
}

type HistoryEntry struct {
	Display        string                 `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
	Timestamp      int64                  `json:"timestamp"`
	Project        string                 `json:"project"`
	SessionID      string                 `json:"sessionId"`
}

type ConversationLine struct {
	Type           string          `json:"type"`
	Message        *MessagePayload `json:"message"`
	SessionID      string          `json:"sessionId"`
	Timestamp      json.RawMessage `json:"timestamp"`
	Cwd            string          `json:"cwd"`
	Slug           string          `json:"slug"`
	Version        string          `json:"version"`
	GitBranch      string          `json:"gitBranch"`
	UUID           string          `json:"uuid"`
	ParentUUID     string          `json:"parentUuid"`
	PermissionMode string          `json:"permissionMode"`
	RequestID      string          `json:"requestId"`
}

type MessagePayload struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
	ID      string          `json:"id"`
	Usage   *UsageInfo      `json:"usage"`
}

type ContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	ID       string          `json:"id"`
	Thinking string          `json:"thinking"`
	Content  json.RawMessage `json:"content"`
}

type UsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type SessionMeta struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"`
	Version    string `json:"version"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	Status     string `json:"status"`
	UpdatedAt  int64  `json:"updatedAt"`
	Name       string `json:"name"`
}

type StatsCache struct {
	Version          int          `json:"version"`
	LastComputedDate string       `json:"lastComputedDate"`
	DailyActivity    []DailyStats `json:"dailyActivity"`
}

type DailyStats struct {
	Date          string `json:"date"`
	MessageCount  int    `json:"messageCount"`
	SessionCount  int    `json:"sessionCount"`
	ToolCallCount int    `json:"toolCallCount"`
}

type FileInventory struct {
	HistoryFiles      []string
	ConversationFiles []string
	MemoryIndexFiles  []string
	MemoryFiles       []string
	SessionMemFiles   []string
	SessionMetaFiles  []string
	StatsCacheFile    string
	ProjectDirs       map[string]string // encoded name -> full path
}

type OverviewStats struct {
	ClaudeDir          string
	TotalSizeBytes     int64
	ConvSizeBytes      int64
	ConversationCount  int
	ProjectCount       int
	SessionCount       int
	MessageCount       int
	ToolCallCount      int
	Tokens             TokenTotals
	DateRange          [2]string
	MemoryFileCount    int
	MemorySizeBytes    int64
	ActiveSessionCount int
	StatsCacheUsed     bool
	StatsCacheThrough  string
}

type TokenTotals struct {
	Input         int64
	Output        int64
	CacheCreation int64
	CacheRead     int64
}

type ProjectStats struct {
	EncodedName       string
	DisplayPath       string
	ConversationCount int
	SizeBytes         int64
	MessageCount      int
	ToolCallCount     int
	MemoryFileCount   int
	DateRange         [2]string
}

type SessionInfo struct {
	SessionID    string
	Project      string
	Title        string
	StartTime    string
	MessageCount int
	ToolCallCount int
	SizeBytes    int64
	Model        string
	Version      string
}

type StorageCategory struct {
	Name      string
	Path      string
	SizeBytes int64
	FileCount int
}

type MemoryInfo struct {
	Project     string
	FilePath    string
	Name        string
	Description string
	MemType     string
	SizeBytes   int64
	IsIndex     bool
	SessionID   string
}

type SecretFinding struct {
	ConversationFile string `json:"file"`
	SessionID        string `json:"sessionId"`
	Project          string `json:"project"`
	PatternName      string `json:"pattern"`
	Match            string `json:"match"`
	RawMatch         string `json:"rawMatch,omitempty"`
	Timestamp        string `json:"timestamp"`
}
