package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/robzolkos/claude-session-export/internal/session"
)

const (
	// PromptsPerPage is the number of prompts per page
	PromptsPerPage = 5
	// MaxTruncatedLength is the max length for truncated content
	MaxTruncatedLength = 2000
)

// RenderOptions contains options for rendering
type RenderOptions struct {
	RepoURL string
}

// RenderMessage renders a single message to HTML
func RenderMessage(msg session.MessageEntry, msgIndex int, opts *RenderOptions) string {
	var buf bytes.Buffer

	roleClass := msg.Role
	roleLabel := capitalizeFirst(msg.Role)

	timestamp := ""
	if !msg.Timestamp.IsZero() {
		timestamp = msg.Timestamp.Format(time.RFC3339)
	}

	msgID := fmt.Sprintf("msg-%d", msgIndex)
	buf.WriteString(fmt.Sprintf(`<div class="message %s" id="%s">`, roleClass, msgID))
	buf.WriteString(fmt.Sprintf(`<div class="message-header">%s`, roleLabel))
	if timestamp != "" {
		buf.WriteString(fmt.Sprintf(` <span class="timestamp">%s</span>`, formatTimestamp(msg.Timestamp)))
	}
	buf.WriteString(`</div>`)
	buf.WriteString(`<div class="message-content">`)

	for _, block := range msg.Content {
		buf.WriteString(RenderContentBlock(block, opts))
	}

	buf.WriteString(`</div></div>`)
	return buf.String()
}

// RenderContentBlock renders a content block to HTML
func RenderContentBlock(block session.ContentBlock, opts *RenderOptions) string {
	switch block.Type {
	case "text":
		return renderText(block)
	case "tool_use":
		return renderToolUse(block, opts)
	case "tool_result":
		return renderToolResult(block, opts)
	case "thinking":
		return renderThinking(block)
	case "image":
		return renderImage(block)
	default:
		return ""
	}
}

func renderText(block session.ContentBlock) string {
	if block.Text == "" {
		return ""
	}
	return fmt.Sprintf(`<div class="text-block">%s</div>`, MarkdownToHTML(block.Text))
}

func renderThinking(block session.ContentBlock) string {
	if block.Text == "" {
		return ""
	}
	return fmt.Sprintf(`<div class="thinking-block">
		<div class="thinking-label">Thinking</div>
		<div class="thinking-content">%s</div>
	</div>`, MarkdownToHTML(block.Text))
}

func renderImage(block session.ContentBlock) string {
	if block.Source == nil {
		return ""
	}
	return fmt.Sprintf(`<div class="image-block">
		<img src="data:%s;base64,%s" alt="Image" style="max-width: 100%%;">
	</div>`, block.Source.MediaType, block.Source.Data)
}

func renderToolUse(block session.ContentBlock, opts *RenderOptions) string {
	input, err := session.ParseToolInput(block.Input)
	if err != nil {
		return renderGenericToolUse(block)
	}

	switch block.Name {
	case "Bash":
		return renderBashTool(block, input)
	case "Write":
		return renderWriteTool(block, input)
	case "Edit", "MultiEdit":
		return renderEditTool(block, input)
	case "TodoWrite":
		return renderTodoTool(block, input)
	case "Read":
		return renderReadTool(block, input)
	case "Glob", "Grep":
		return renderSearchTool(block, input)
	default:
		return renderGenericToolUse(block)
	}
}

func renderBashTool(block session.ContentBlock, input *session.ToolInput) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block bash-tool">`)
	buf.WriteString(`<div class="tool-header">Bash`)
	if input.Description != "" {
		buf.WriteString(fmt.Sprintf(` <span class="tool-description">%s</span>`, html.EscapeString(input.Description)))
	}
	buf.WriteString(`</div>`)
	buf.WriteString(fmt.Sprintf(`<pre class="command"><code>%s</code></pre>`, html.EscapeString(input.Command)))
	buf.WriteString(`</div>`)
	return buf.String()
}

func renderWriteTool(block session.ContentBlock, input *session.ToolInput) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block write-tool">`)
	buf.WriteString(fmt.Sprintf(`<div class="tool-header">Write: %s</div>`, html.EscapeString(input.FilePath)))

	content := input.Content
	truncated := false
	if len(content) > MaxTruncatedLength {
		content = content[:MaxTruncatedLength]
		truncated = true
	}

	buf.WriteString(`<pre class="file-content"><code>`)
	buf.WriteString(html.EscapeString(content))
	if truncated {
		buf.WriteString(`<span class="truncated">... (truncated)</span>`)
	}
	buf.WriteString(`</code></pre>`)
	buf.WriteString(`</div>`)
	return buf.String()
}

func renderEditTool(block session.ContentBlock, input *session.ToolInput) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block edit-tool">`)
	buf.WriteString(fmt.Sprintf(`<div class="tool-header">Edit: %s</div>`, html.EscapeString(input.FilePath)))

	buf.WriteString(`<div class="edit-diff">`)
	if input.OldString != "" {
		buf.WriteString(`<div class="diff-old">`)
		buf.WriteString(`<div class="diff-label">Old</div>`)
		buf.WriteString(fmt.Sprintf(`<pre><code>%s</code></pre>`, html.EscapeString(input.OldString)))
		buf.WriteString(`</div>`)
	}
	if input.NewString != "" {
		buf.WriteString(`<div class="diff-new">`)
		buf.WriteString(`<div class="diff-label">New</div>`)
		buf.WriteString(fmt.Sprintf(`<pre><code>%s</code></pre>`, html.EscapeString(input.NewString)))
		buf.WriteString(`</div>`)
	}
	buf.WriteString(`</div>`)
	buf.WriteString(`</div>`)
	return buf.String()
}

func renderReadTool(block session.ContentBlock, input *session.ToolInput) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block read-tool">`)
	buf.WriteString(fmt.Sprintf(`<div class="tool-header">Read: %s</div>`, html.EscapeString(input.FilePath)))
	buf.WriteString(`</div>`)
	return buf.String()
}

func renderSearchTool(block session.ContentBlock, input *session.ToolInput) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block search-tool">`)
	buf.WriteString(fmt.Sprintf(`<div class="tool-header">%s</div>`, block.Name))

	if input.Pattern != "" {
		buf.WriteString(fmt.Sprintf(`<div class="search-pattern">Pattern: <code>%s</code></div>`, html.EscapeString(input.Pattern)))
	}
	if input.Path != "" {
		buf.WriteString(fmt.Sprintf(`<div class="search-path">Path: <code>%s</code></div>`, html.EscapeString(input.Path)))
	}

	buf.WriteString(`</div>`)
	return buf.String()
}

func renderTodoTool(block session.ContentBlock, input *session.ToolInput) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block todo-tool">`)
	buf.WriteString(`<div class="tool-header">TodoWrite</div>`)
	buf.WriteString(`<ul class="todo-list">`)

	for _, todo := range input.Todos {
		statusClass := "todo-" + todo.Status
		statusIcon := "○"
		switch todo.Status {
		case "completed":
			statusIcon = "✓"
		case "in_progress":
			statusIcon = "◐"
		}
		buf.WriteString(fmt.Sprintf(`<li class="%s"><span class="todo-status">%s</span> %s</li>`,
			statusClass, statusIcon, html.EscapeString(todo.Content)))
	}

	buf.WriteString(`</ul></div>`)
	return buf.String()
}

func renderGenericToolUse(block session.ContentBlock) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="tool-block">`)
	buf.WriteString(fmt.Sprintf(`<div class="tool-header">%s</div>`, html.EscapeString(block.Name)))

	if len(block.Input) > 0 {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, block.Input, "", "  "); err == nil {
			content := prettyJSON.String()
			if len(content) > MaxTruncatedLength {
				content = content[:MaxTruncatedLength] + "..."
			}
			buf.WriteString(fmt.Sprintf(`<pre class="tool-input"><code>%s</code></pre>`, html.EscapeString(content)))
		}
	}

	buf.WriteString(`</div>`)
	return buf.String()
}

func renderToolResult(block session.ContentBlock, opts *RenderOptions) string {
	var buf bytes.Buffer

	isError := block.IsError
	resultClass := "tool-result"
	if isError {
		resultClass += " tool-error"
	}

	buf.WriteString(fmt.Sprintf(`<div class="%s">`, resultClass))

	content := extractToolResultContent(block.Content)
	if content != "" {
		// Check for commit pattern
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if matches := session.CommitPattern.FindStringSubmatch(line); len(matches) > 2 {
				buf.WriteString(renderCommitCard(matches[1], matches[2], opts))
			}
		}

		// Truncate if needed
		if len(content) > MaxTruncatedLength {
			content = content[:MaxTruncatedLength] + "..."
		}
		buf.WriteString(fmt.Sprintf(`<pre class="tool-output"><code>%s</code></pre>`, html.EscapeString(content)))
	}

	buf.WriteString(`</div>`)
	return buf.String()
}

func renderCommitCard(hash, message string, opts *RenderOptions) string {
	var buf bytes.Buffer
	buf.WriteString(`<div class="commit-card">`)
	buf.WriteString(`<span class="commit-icon">📦</span>`)

	// Safely truncate hash to 7 characters for display
	hashDisplay := hash
	if len(hash) > 7 {
		hashDisplay = hash[:7]
	}

	if opts != nil && opts.RepoURL != "" {
		commitURL := fmt.Sprintf("%s/commit/%s", opts.RepoURL, hash)
		buf.WriteString(fmt.Sprintf(`<a href="%s" class="commit-hash">%s</a>`, commitURL, hashDisplay))
	} else {
		buf.WriteString(fmt.Sprintf(`<span class="commit-hash">%s</span>`, hashDisplay))
	}

	buf.WriteString(fmt.Sprintf(` <span class="commit-message">%s</span>`, html.EscapeString(message)))
	buf.WriteString(`</div>`)
	return buf.String()
}

func extractToolResultContent(content interface{}) string {
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

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Jan 2, 2006 3:04 PM")
}

// capitalizeFirst capitalizes the first letter of a string
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
