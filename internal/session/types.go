package session

import (
	"encoding/json"
	"time"
)

// Session represents a Claude Code session
type Session struct {
	Messages []Message        `json:"messages"`
	Metadata *SessionMetadata `json:"-"`
}

// Message represents a single message in the conversation
type Message struct {
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Content      Content         `json:"-"`
	RawContent   json.RawMessage `json:"content"`
	Timestamp    time.Time       `json:"-"`
	RawTimestamp string          `json:"timestamp"`

	// New Claude Code format: message is nested
	NestedMessage *NestedMessage `json:"message,omitempty"`

	// Session metadata (from top-level fields)
	Cwd       string `json:"cwd,omitempty"`
	GitBranch string `json:"gitBranch,omitempty"`
	Version   string `json:"version,omitempty"`

	// Model and usage (extracted from nested message)
	Model string
	Usage *TokenUsage
}

// NestedMessage represents the nested message in new Claude Code format
type NestedMessage struct {
	Role       string          `json:"role"`
	RawContent json.RawMessage `json:"content"`
	Model      string          `json:"model,omitempty"`
	Usage      *TokenUsage     `json:"usage,omitempty"`
}

// Content can be a string or array of content blocks
type Content []ContentBlock

// ContentBlock represents a block of content within a message
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Name      string          `json:"name,omitempty"`
	ID        string          `json:"id,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   interface{}     `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// For images
	Source *ImageSource `json:"source,omitempty"`
}

// ImageSource represents an image source in a content block
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ToolInput represents parsed tool input
type ToolInput struct {
	Command     string     `json:"command,omitempty"`
	Description string     `json:"description,omitempty"`
	FilePath    string     `json:"file_path,omitempty"`
	Content     string     `json:"content,omitempty"`
	OldString   string     `json:"old_string,omitempty"`
	NewString   string     `json:"new_string,omitempty"`
	Pattern     string     `json:"pattern,omitempty"`
	Path        string     `json:"path,omitempty"`
	Todos       []TodoItem `json:"todos,omitempty"`
}

// TodoItem represents a todo item in the TodoWrite tool
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// Conversation represents a grouped conversation starting with a user message
type Conversation struct {
	UserText       string
	Timestamp      time.Time
	Messages       []MessageEntry
	IsContinuation bool
}

// MessageEntry represents a message with its metadata
type MessageEntry struct {
	Role      string
	Content   Content
	Timestamp time.Time
	Model     string
	Usage     *TokenUsage
}

// TokenUsage represents token usage statistics for a message
type TokenUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_input_tokens"`
	CacheWriteTokens int `json:"cache_creation_input_tokens"`
}

// SessionMetadata contains metadata about the session
type SessionMetadata struct {
	Cwd          string
	GitBranch    string
	Version      string
	Models       []string
	TotalInput   int
	TotalOutput  int
	TotalCache   int
	StartTime    time.Time
	EndTime      time.Time
	ActiveTime   time.Duration // Time excluding gaps > threshold
}

// ToolStats tracks statistics about tool usage
type ToolStats struct {
	BashCount  int
	ReadCount  int
	WriteCount int
	EditCount  int
	GlobCount  int
	GrepCount  int
	OtherCount int
}

// IndexItem represents an item in the index (prompt or commit)
type IndexItem struct {
	Type      string // "prompt" or "commit"
	Timestamp time.Time
	Text      string
	PageNum   int
	MessageID string
	Stats     *ToolStats
	LongTexts []string

	// For commits
	CommitHash    string
	CommitMessage string
	RepoURL       string
}
