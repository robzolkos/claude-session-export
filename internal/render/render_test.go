package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/robzolkos/claude-session-export/internal/session"
)

func TestRenderMessage(t *testing.T) {
	msg := session.MessageEntry{
		Role: "user",
		Content: session.Content{
			{Type: "text", Text: "Hello, Claude!"},
		},
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	html := RenderMessage(msg, 0, nil)

	if !strings.Contains(html, `class="message user"`) {
		t.Error("Expected message to have user class")
	}

	if !strings.Contains(html, "Hello, Claude!") {
		t.Error("Expected message content to be present")
	}

	if !strings.Contains(html, "User") {
		t.Error("Expected role label to be present")
	}
}

func TestRenderContentBlock_Text(t *testing.T) {
	block := session.ContentBlock{
		Type: "text",
		Text: "This is a **bold** text.",
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "text-block") {
		t.Error("Expected text-block class")
	}

	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Error("Expected markdown bold to be rendered")
	}
}

func TestRenderContentBlock_Thinking(t *testing.T) {
	block := session.ContentBlock{
		Type: "thinking",
		Text: "Let me think about this...",
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "thinking-block") {
		t.Error("Expected thinking-block class")
	}

	if !strings.Contains(html, "Thinking") {
		t.Error("Expected thinking label")
	}
}

func TestRenderContentBlock_ToolUse_Bash(t *testing.T) {
	block := session.ContentBlock{
		Type:  "tool_use",
		Name:  "Bash",
		Input: json.RawMessage(`{"command": "ls -la", "description": "List files"}`),
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "bash-tool") {
		t.Error("Expected bash-tool class")
	}

	if !strings.Contains(html, "ls -la") {
		t.Error("Expected command to be present")
	}

	if !strings.Contains(html, "List files") {
		t.Error("Expected description to be present")
	}
}

func TestRenderContentBlock_ToolUse_Write(t *testing.T) {
	block := session.ContentBlock{
		Type:  "tool_use",
		Name:  "Write",
		Input: json.RawMessage(`{"file_path": "/path/to/file.go", "content": "package main\n\nfunc main() {}"}`),
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "write-tool") {
		t.Error("Expected write-tool class")
	}

	if !strings.Contains(html, "/path/to/file.go") {
		t.Error("Expected file path to be present")
	}

	if !strings.Contains(html, "package main") {
		t.Error("Expected content to be present")
	}
}

func TestRenderContentBlock_ToolUse_Edit(t *testing.T) {
	block := session.ContentBlock{
		Type:  "tool_use",
		Name:  "Edit",
		Input: json.RawMessage(`{"file_path": "/path/to/file.go", "old_string": "old code", "new_string": "new code"}`),
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "edit-tool") {
		t.Error("Expected edit-tool class")
	}

	if !strings.Contains(html, "diff-old") {
		t.Error("Expected diff-old section")
	}

	if !strings.Contains(html, "diff-new") {
		t.Error("Expected diff-new section")
	}

	if !strings.Contains(html, "old code") {
		t.Error("Expected old string to be present")
	}

	if !strings.Contains(html, "new code") {
		t.Error("Expected new string to be present")
	}
}

func TestRenderContentBlock_ToolUse_TodoWrite(t *testing.T) {
	block := session.ContentBlock{
		Type: "tool_use",
		Name: "TodoWrite",
		Input: json.RawMessage(`{"todos": [
			{"content": "Task 1", "status": "pending"},
			{"content": "Task 2", "status": "in_progress"},
			{"content": "Task 3", "status": "completed"}
		]}`),
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "todo-tool") {
		t.Error("Expected todo-tool class")
	}

	if !strings.Contains(html, "todo-pending") {
		t.Error("Expected todo-pending class")
	}

	if !strings.Contains(html, "todo-in_progress") {
		t.Error("Expected todo-in_progress class")
	}

	if !strings.Contains(html, "todo-completed") {
		t.Error("Expected todo-completed class")
	}

	if !strings.Contains(html, "✓") {
		t.Error("Expected completed checkmark")
	}
}

func TestRenderContentBlock_Image(t *testing.T) {
	block := session.ContentBlock{
		Type: "image",
		Source: &session.ImageSource{
			Type:      "base64",
			MediaType: "image/png",
			Data:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		},
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "image-block") {
		t.Error("Expected image-block class")
	}

	if !strings.Contains(html, "data:image/png;base64,") {
		t.Error("Expected base64 image data URI")
	}
}

func TestRenderContentBlock_ToolResult(t *testing.T) {
	block := session.ContentBlock{
		Type:    "tool_result",
		Content: "Command output here",
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "tool-result") {
		t.Error("Expected tool-result class")
	}

	if !strings.Contains(html, "Command output here") {
		t.Error("Expected tool output content")
	}
}

func TestRenderContentBlock_ToolResult_WithCommit(t *testing.T) {
	block := session.ContentBlock{
		Type:    "tool_result",
		Content: "[main abc1234] Initial commit",
	}

	opts := &RenderOptions{RepoURL: "https://github.com/user/repo"}
	html := RenderContentBlock(block, opts)

	if !strings.Contains(html, "commit-card") {
		t.Error("Expected commit-card")
	}

	if !strings.Contains(html, "abc1234") {
		t.Error("Expected commit hash")
	}

	if !strings.Contains(html, "https://github.com/user/repo/commit/abc1234") {
		t.Error("Expected GitHub commit link")
	}
}

func TestRenderContentBlock_ToolResult_Error(t *testing.T) {
	block := session.ContentBlock{
		Type:    "tool_result",
		Content: "Error: command failed",
		IsError: true,
	}

	html := RenderContentBlock(block, nil)

	if !strings.Contains(html, "tool-error") {
		t.Error("Expected tool-error class for error results")
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a long text", 10, "this is a ..."},
		{"exact len", 9, "exact len"},
	}

	for _, tt := range tests {
		result := TruncateText(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncateText(%q, %d) = %q, expected %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<script>alert('xss')</script>", "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&#34;quoted&#34;"},
	}

	for _, tt := range tests {
		result := EscapeHTML(tt.input)
		if result != tt.expected {
			t.Errorf("EscapeHTML(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}
