package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseJSON(t *testing.T) {
	data := []byte(`{
		"messages": [
			{
				"role": "user",
				"content": "Hello, Claude!",
				"timestamp": "2024-01-15T10:30:00Z"
			},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Hello! How can I help you today?"}
				],
				"timestamp": "2024-01-15T10:30:05Z"
			}
		]
	}`)

	session, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(session.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(session.Messages))
	}

	if session.Messages[0].Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", session.Messages[0].Role)
	}

	if len(session.Messages[0].Content) != 1 {
		t.Errorf("Expected 1 content block, got %d", len(session.Messages[0].Content))
	}

	if session.Messages[0].Content[0].Text != "Hello, Claude!" {
		t.Errorf("Expected 'Hello, Claude!', got '%s'", session.Messages[0].Content[0].Text)
	}
}

func TestParseJSONL(t *testing.T) {
	data := []byte(`{"role": "user", "content": "First message", "timestamp": "2024-01-15T10:00:00Z"}
{"role": "assistant", "content": [{"type": "text", "text": "Response"}], "timestamp": "2024-01-15T10:00:05Z"}
{"role": "user", "content": "Second message", "timestamp": "2024-01-15T10:01:00Z"}`)

	session, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(session.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(session.Messages))
	}

	if session.Messages[0].Content[0].Text != "First message" {
		t.Errorf("Expected 'First message', got '%s'", session.Messages[0].Content[0].Text)
	}
}

func TestParseJSONLWithSummary(t *testing.T) {
	// JSONL files may contain non-message entries like summaries
	data := []byte(`{"type": "summary", "summary": "This is a summary"}
{"role": "user", "content": "Hello", "timestamp": "2024-01-15T10:00:00Z"}
{"role": "assistant", "content": [{"type": "text", "text": "Hi"}], "timestamp": "2024-01-15T10:00:05Z"}`)

	session, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should skip the summary entry
	if len(session.Messages) != 2 {
		t.Errorf("Expected 2 messages (excluding summary), got %d", len(session.Messages))
	}
}

func TestParseEmptyData(t *testing.T) {
	session, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(session.Messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(session.Messages))
	}
}

func TestParseToolUse(t *testing.T) {
	data := []byte(`{
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"name": "Bash",
						"id": "tool_123",
						"input": {"command": "ls -la", "description": "List files"}
					}
				],
				"timestamp": "2024-01-15T10:30:00Z"
			}
		]
	}`)

	session, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(session.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(session.Messages))
	}

	if len(session.Messages[0].Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(session.Messages[0].Content))
	}

	block := session.Messages[0].Content[0]
	if block.Type != "tool_use" {
		t.Errorf("Expected type 'tool_use', got '%s'", block.Type)
	}

	if block.Name != "Bash" {
		t.Errorf("Expected name 'Bash', got '%s'", block.Name)
	}

	input, err := ParseToolInput(block.Input)
	if err != nil {
		t.Fatalf("ParseToolInput failed: %v", err)
	}

	if input.Command != "ls -la" {
		t.Errorf("Expected command 'ls -la', got '%s'", input.Command)
	}
}

func TestExtractText(t *testing.T) {
	msg := Message{
		Content: Content{
			{Type: "text", Text: "First part"},
			{Type: "tool_use", Name: "Bash"},
			{Type: "text", Text: "Second part"},
		},
	}

	text := ExtractText(&msg)
	expected := "First part\nSecond part"
	if text != expected {
		t.Errorf("Expected '%s', got '%s'", expected, text)
	}
}

func TestGroupConversations(t *testing.T) {
	now := time.Now()
	session := &Session{
		Messages: []Message{
			{Role: "user", Content: Content{{Type: "text", Text: "Q1"}}, Timestamp: now},
			{Role: "assistant", Content: Content{{Type: "text", Text: "A1"}}, Timestamp: now.Add(time.Second)},
			{Role: "user", Content: Content{{Type: "text", Text: "Q2"}}, Timestamp: now.Add(time.Minute)},
			{Role: "assistant", Content: Content{{Type: "text", Text: "A2"}}, Timestamp: now.Add(time.Minute + time.Second)},
		},
	}

	convs := GroupConversations(session)

	if len(convs) != 2 {
		t.Fatalf("Expected 2 conversations, got %d", len(convs))
	}

	if convs[0].UserText != "Q1" {
		t.Errorf("Expected UserText 'Q1', got '%s'", convs[0].UserText)
	}

	if len(convs[0].Messages) != 2 {
		t.Errorf("Expected 2 messages in first conversation, got %d", len(convs[0].Messages))
	}
}

func TestAnalyzeConversation(t *testing.T) {
	conv := &Conversation{
		Messages: []MessageEntry{
			{
				Content: Content{
					{Type: "tool_use", Name: "Bash"},
					{Type: "tool_use", Name: "Read"},
					{Type: "tool_use", Name: "Bash"},
					{Type: "tool_use", Name: "Write"},
				},
			},
		},
	}

	stats, _ := AnalyzeConversation(conv)

	if stats.BashCount != 2 {
		t.Errorf("Expected BashCount 2, got %d", stats.BashCount)
	}

	if stats.ReadCount != 1 {
		t.Errorf("Expected ReadCount 1, got %d", stats.ReadCount)
	}

	if stats.WriteCount != 1 {
		t.Errorf("Expected WriteCount 1, got %d", stats.WriteCount)
	}
}

func TestCommitPattern(t *testing.T) {
	tests := []struct {
		input       string
		wantMatch   bool
		wantHash    string
		wantMessage string
	}{
		{"[main abc1234] Initial commit", true, "abc1234", "Initial commit"},
		{"[feature/test def5678] Add feature", true, "def5678", "Add feature"},
		{"[main 1234567890] Long hash", true, "1234567890", "Long hash"},
		{"Not a commit line", false, "", ""},
		{"[main abc] Too short hash", false, "", ""},
	}

	for _, tt := range tests {
		matches := CommitPattern.FindStringSubmatch(tt.input)
		if tt.wantMatch {
			if len(matches) < 3 {
				t.Errorf("Expected match for '%s', got none", tt.input)
				continue
			}
			if matches[1] != tt.wantHash {
				t.Errorf("Expected hash '%s', got '%s'", tt.wantHash, matches[1])
			}
			if matches[2] != tt.wantMessage {
				t.Errorf("Expected message '%s', got '%s'", tt.wantMessage, matches[2])
			}
		} else {
			if len(matches) >= 3 {
				t.Errorf("Expected no match for '%s', got %v", tt.input, matches)
			}
		}
	}
}

func TestGitHubRepoPattern(t *testing.T) {
	tests := []struct {
		input     string
		wantMatch bool
		wantRepo  string
	}{
		{"git@github.com:user/repo.git", true, "user/repo"},
		{"https://github.com/user/repo.git", true, "user/repo"},
		{"github.com/user/repo ", true, "user/repo"},
		{"gitlab.com/user/repo", false, ""},
	}

	for _, tt := range tests {
		matches := GitHubRepoPattern.FindStringSubmatch(tt.input)
		if tt.wantMatch {
			if len(matches) < 2 {
				t.Errorf("Expected match for '%s', got none", tt.input)
				continue
			}
			repo := matches[1]
			repo = repo[:len(repo)-len(".git")] // Remove .git if present
			if len(repo) > len(tt.wantRepo) {
				repo = repo[:len(tt.wantRepo)]
			}
		} else {
			if len(matches) >= 2 {
				t.Errorf("Expected no match for '%s', got %v", tt.input, matches)
			}
		}
	}
}

func TestParseToolInput(t *testing.T) {
	input := json.RawMessage(`{
		"command": "git status",
		"description": "Check git status",
		"file_path": "/path/to/file",
		"todos": [
			{"content": "Task 1", "status": "pending"},
			{"content": "Task 2", "status": "completed"}
		]
	}`)

	parsed, err := ParseToolInput(input)
	if err != nil {
		t.Fatalf("ParseToolInput failed: %v", err)
	}

	if parsed.Command != "git status" {
		t.Errorf("Expected command 'git status', got '%s'", parsed.Command)
	}

	if parsed.Description != "Check git status" {
		t.Errorf("Expected description 'Check git status', got '%s'", parsed.Description)
	}

	if len(parsed.Todos) != 2 {
		t.Errorf("Expected 2 todos, got %d", len(parsed.Todos))
	}

	if parsed.Todos[0].Status != "pending" {
		t.Errorf("Expected first todo status 'pending', got '%s'", parsed.Todos[0].Status)
	}
}

func TestGetFirstUserMessage(t *testing.T) {
	session := &Session{
		Messages: []Message{
			{Role: "assistant", Content: Content{{Type: "text", Text: "Hello"}}},
			{Role: "user", Content: Content{{Type: "text", Text: "First user message"}}},
			{Role: "user", Content: Content{{Type: "text", Text: "Second user message"}}},
		},
	}

	first := GetFirstUserMessage(session)
	if first != "First user message" {
		t.Errorf("Expected 'First user message', got '%s'", first)
	}
}

func TestIsJSONL(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"session.jsonl", true},
		{"session.JSONL", true},
		{"session.json", false},
		{"session.txt", false},
	}

	for _, tt := range tests {
		result := IsJSONL(tt.path)
		if result != tt.expected {
			t.Errorf("IsJSONL(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}
