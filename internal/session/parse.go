package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CommitPattern matches git commit output like "[main abc1234] commit message"
var CommitPattern = regexp.MustCompile(`\[[\w\-/]+\s+([a-f0-9]{7,})\]\s+(.+)`)

// GitHubRepoPattern matches GitHub URLs in git output
var GitHubRepoPattern = regexp.MustCompile(`github\.com[:/]([^/]+/[^/\s]+?)(?:\.git)?(?:\s|$)`)

// ParseFile parses a session file (JSON or JSONL format)
func ParseFile(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return Parse(data)
}

// Parse parses session data from bytes
func Parse(data []byte) (*Session, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return &Session{}, nil
	}

	// Detect JSONL: multiple lines each starting with {
	if isJSONL(data) {
		return parseJSONL(data)
	}

	// Otherwise treat as JSON
	return parseJSON(data)
}

// isJSONL checks if the data appears to be JSONL format
func isJSONL(data []byte) bool {
	// If it starts with [, it's definitely JSON (array)
	if data[0] == '[' {
		return false
	}

	// If it starts with { and contains a "messages" key near the start,
	// it's likely a JSON session object, not JSONL
	if data[0] == '{' {
		// Quick check: try to parse as JSON object with messages
		var test struct {
			Messages json.RawMessage `json:"messages"`
		}
		if json.Unmarshal(data, &test) == nil && len(test.Messages) > 0 {
			return false
		}
	}

	// Check if we have multiple lines each starting with {
	lines := bytes.Split(data, []byte("\n"))
	jsonLineCount := 0
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if line[0] == '{' {
			jsonLineCount++
		}
	}

	// If we have more than one JSON line, treat as JSONL
	return jsonLineCount > 1
}

func parseJSON(data []byte) (*Session, error) {
	// Try parsing as a session object with messages array
	var session Session
	if err := json.Unmarshal(data, &session); err == nil && len(session.Messages) > 0 {
		if err := parseMessages(&session); err != nil {
			return nil, err
		}
		return &session, nil
	}

	// Try parsing as an array of messages directly
	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	session = Session{Messages: messages}
	if err := parseMessages(&session); err != nil {
		return nil, err
	}
	return &session, nil
}

func parseJSONL(data []byte) (*Session, error) {
	var messages []Message
	scanner := bufio.NewScanner(bytes.NewReader(data))

	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line size

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			// Skip invalid lines silently (matches Python behavior)
			continue
		}

		// Skip non-message types (like "summary" entries)
		if msg.Type != "" && msg.Type != "message" {
			continue
		}

		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning JSONL: %w", err)
	}

	session := &Session{Messages: messages}
	if err := parseMessages(session); err != nil {
		return nil, err
	}
	return session, nil
}

func parseMessages(session *Session) error {
	for i := range session.Messages {
		msg := &session.Messages[i]

		// Parse timestamp
		if msg.RawTimestamp != "" {
			t, err := time.Parse(time.RFC3339, msg.RawTimestamp)
			if err != nil {
				t, err = time.Parse(time.RFC3339Nano, msg.RawTimestamp)
			}
			if err == nil {
				msg.Timestamp = t
			}
		}

		// Parse content
		content, err := parseContent(msg.RawContent)
		if err != nil {
			return fmt.Errorf("parsing content: %w", err)
		}
		msg.Content = content
	}
	return nil
}

func parseContent(raw json.RawMessage) (Content, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Try parsing as string first
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return Content{{Type: "text", Text: str}}, nil
	}

	// Try parsing as array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// ParseToolInput parses tool input from raw JSON
func ParseToolInput(raw json.RawMessage) (*ToolInput, error) {
	var input ToolInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

// ExtractText extracts all text content from a message
func ExtractText(msg *Message) string {
	var texts []string
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// GroupConversations groups messages into conversations starting with user messages
func GroupConversations(session *Session) []Conversation {
	var conversations []Conversation
	var current *Conversation

	for _, msg := range session.Messages {
		if msg.Role == "user" {
			// Start a new conversation
			if current != nil {
				conversations = append(conversations, *current)
			}
			current = &Conversation{
				UserText:  ExtractText(&msg),
				Timestamp: msg.Timestamp,
				Messages: []MessageEntry{{
					Role:      msg.Role,
					Content:   msg.Content,
					Timestamp: msg.Timestamp,
				}},
			}
		} else if current != nil {
			current.Messages = append(current.Messages, MessageEntry{
				Role:      msg.Role,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			})
		}
	}

	if current != nil {
		conversations = append(conversations, *current)
	}

	return conversations
}

// AnalyzeConversation analyzes a conversation for tool usage statistics
func AnalyzeConversation(conv *Conversation) (*ToolStats, []string) {
	stats := &ToolStats{}
	var longTexts []string

	for _, msg := range conv.Messages {
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				switch block.Name {
				case "Bash":
					stats.BashCount++
				case "Read":
					stats.ReadCount++
				case "Write":
					stats.WriteCount++
				case "Edit", "MultiEdit":
					stats.EditCount++
				case "Glob":
					stats.GlobCount++
				case "Grep":
					stats.GrepCount++
				default:
					stats.OtherCount++
				}
			}

			// Collect long text blocks (300+ chars)
			if block.Type == "text" && len(block.Text) >= 300 {
				longTexts = append(longTexts, block.Text)
			}
		}
	}

	return stats, longTexts
}

// DetectGitHubRepo detects GitHub repository from bash tool output
func DetectGitHubRepo(session *Session) string {
	for _, msg := range session.Messages {
		for _, block := range msg.Content {
			if block.Type == "tool_result" {
				content := extractToolResultText(block.Content)
				if matches := GitHubRepoPattern.FindStringSubmatch(content); len(matches) > 1 {
					repo := strings.TrimSuffix(matches[1], ".git")
					return "https://github.com/" + repo
				}
			}
		}
	}
	return ""
}

func extractToolResultText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// ExtractCommits extracts git commits from bash tool output
func ExtractCommits(session *Session) []IndexItem {
	var commits []IndexItem

	for _, msg := range session.Messages {
		for _, block := range msg.Content {
			if block.Type == "tool_result" {
				content := extractToolResultText(block.Content)
				for _, line := range strings.Split(content, "\n") {
					if matches := CommitPattern.FindStringSubmatch(line); len(matches) > 2 {
						commits = append(commits, IndexItem{
							Type:          "commit",
							Timestamp:     msg.Timestamp,
							CommitHash:    matches[1],
							CommitMessage: matches[2],
						})
					}
				}
			}
		}
	}

	return commits
}

// IsJSONL checks if a file is JSONL format
func IsJSONL(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".jsonl"
}

// ParseReader parses session data from a reader
func ParseReader(r io.Reader) (*Session, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// GetFirstUserMessage returns the first user message text
func GetFirstUserMessage(session *Session) string {
	for _, msg := range session.Messages {
		if msg.Role == "user" {
			text := ExtractText(&msg)
			if text != "" {
				return text
			}
		}
	}
	return ""
}
