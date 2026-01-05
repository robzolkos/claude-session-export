package render

import (
	"strings"
	"testing"
)

func TestMarkdownToHTML_Text(t *testing.T) {
	input := "This is plain text."
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<p>") {
		t.Error("Expected paragraph tag")
	}

	if !strings.Contains(result, "This is plain text.") {
		t.Error("Expected text content to be preserved")
	}
}

func TestMarkdownToHTML_Bold(t *testing.T) {
	input := "This is **bold** text."
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Errorf("Expected bold to be rendered, got: %s", result)
	}
}

func TestMarkdownToHTML_Italic(t *testing.T) {
	input := "This is *italic* text."
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<em>italic</em>") {
		t.Errorf("Expected italic to be rendered, got: %s", result)
	}
}

func TestMarkdownToHTML_InlineCode(t *testing.T) {
	input := "Use `code` here."
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<code>code</code>") {
		t.Errorf("Expected inline code to be rendered, got: %s", result)
	}
}

func TestMarkdownToHTML_FencedCodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```"
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<pre><code") {
		t.Error("Expected code block")
	}

	if !strings.Contains(result, `class="language-go"`) {
		t.Error("Expected language class")
	}

	if !strings.Contains(result, "func main()") {
		t.Error("Expected code content")
	}
}

func TestMarkdownToHTML_FencedCodeBlock_NoLanguage(t *testing.T) {
	input := "```\nplain code\n```"
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<pre><code>") {
		t.Error("Expected code block without language class")
	}

	if strings.Contains(result, "language-") {
		t.Error("Expected no language class for plain code block")
	}
}

func TestMarkdownToHTML_Link(t *testing.T) {
	input := "Click [here](https://example.com) for more."
	result := MarkdownToHTML(input)

	if !strings.Contains(result, `<a href="https://example.com">here</a>`) {
		t.Errorf("Expected link to be rendered, got: %s", result)
	}
}

func TestMarkdownToHTML_Headers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"# Header 1", "<h1>Header 1</h1>"},
		{"## Header 2", "<h2>Header 2</h2>"},
		{"### Header 3", "<h3>Header 3</h3>"},
	}

	for _, tt := range tests {
		result := MarkdownToHTML(tt.input)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("MarkdownToHTML(%q) expected to contain %q, got: %s", tt.input, tt.expected, result)
		}
	}
}

func TestMarkdownToHTML_UnorderedList(t *testing.T) {
	// Using - instead of * to avoid conflict with italic pattern
	input := "- Item 1\n- Item 2"
	result := MarkdownToHTML(input)

	if !strings.Contains(result, "<li>Item 1</li>") {
		t.Errorf("Expected list item, got: %s", result)
	}

	if !strings.Contains(result, "<li>Item 2</li>") {
		t.Errorf("Expected list item, got: %s", result)
	}
}

func TestMarkdownToHTML_Paragraphs(t *testing.T) {
	input := "Paragraph 1.\n\nParagraph 2."
	result := MarkdownToHTML(input)

	// Should have two paragraph tags
	count := strings.Count(result, "<p>")
	if count != 2 {
		t.Errorf("Expected 2 paragraphs, got %d: %s", count, result)
	}
}

func TestMarkdownToHTML_EscapesHTML(t *testing.T) {
	input := "<script>alert('xss')</script>"
	result := MarkdownToHTML(input)

	if strings.Contains(result, "<script>") {
		t.Error("Expected HTML to be escaped")
	}

	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("Expected escaped script tag")
	}
}

func TestMarkdownToHTML_Empty(t *testing.T) {
	result := MarkdownToHTML("")
	if result != "" {
		t.Errorf("Expected empty string for empty input, got: %s", result)
	}
}

func TestMarkdownToHTML_Complex(t *testing.T) {
	input := `# Title

This is a **bold** paragraph with *italic* and ` + "`code`" + `.

## Code Example

` + "```go" + `
func hello() string {
    return "world"
}
` + "```" + `

See [documentation](https://docs.example.com) for more.`

	result := MarkdownToHTML(input)

	checks := []string{
		"<h1>Title</h1>",
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<code>code</code>",
		"<h2>Code Example</h2>",
		`<pre><code class="language-go">`,
		"func hello()",
		`<a href="https://docs.example.com">documentation</a>`,
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("Expected result to contain %q, got: %s", check, result)
		}
	}
}
