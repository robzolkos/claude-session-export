package render

import (
	"html"
	"regexp"
	"strings"
)

var (
	// Fenced code block pattern
	fencedCodePattern = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	// Inline code pattern
	inlineCodePattern = regexp.MustCompile("`([^`]+)`")
	// Bold pattern
	boldPattern = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// Italic pattern
	italicPattern = regexp.MustCompile(`\*([^*]+)\*`)
	// Link pattern
	linkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	// Header patterns
	h1Pattern = regexp.MustCompile(`(?m)^# (.+)$`)
	h2Pattern = regexp.MustCompile(`(?m)^## (.+)$`)
	h3Pattern = regexp.MustCompile(`(?m)^### (.+)$`)
	// List patterns
	ulPattern = regexp.MustCompile(`(?m)^[*-] (.+)$`)
	olPattern = regexp.MustCompile(`(?m)^(\d+)\. (.+)$`)
)

// MarkdownToHTML converts markdown text to HTML
func MarkdownToHTML(text string) string {
	if text == "" {
		return ""
	}

	// Escape HTML first
	text = html.EscapeString(text)

	// Process fenced code blocks
	text = fencedCodePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := fencedCodePattern.FindStringSubmatch(match)
		lang := parts[1]
		code := parts[2]
		if lang != "" {
			return `<pre><code class="language-` + lang + `">` + code + `</code></pre>`
		}
		return `<pre><code>` + code + `</code></pre>`
	})

	// Process inline code
	text = inlineCodePattern.ReplaceAllString(text, `<code>$1</code>`)

	// Process bold
	text = boldPattern.ReplaceAllString(text, `<strong>$1</strong>`)

	// Process italic
	text = italicPattern.ReplaceAllString(text, `<em>$1</em>`)

	// Process links
	text = linkPattern.ReplaceAllString(text, `<a href="$2">$1</a>`)

	// Process headers
	text = h1Pattern.ReplaceAllString(text, `<h1>$1</h1>`)
	text = h2Pattern.ReplaceAllString(text, `<h2>$1</h2>`)
	text = h3Pattern.ReplaceAllString(text, `<h3>$1</h3>`)

	// Process unordered lists
	text = ulPattern.ReplaceAllString(text, `<li>$1</li>`)

	// Process ordered lists
	text = olPattern.ReplaceAllString(text, `<li>$2</li>`)

	// Convert double newlines to paragraphs
	paragraphs := strings.Split(text, "\n\n")
	var result []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Don't wrap elements that are already block-level
		if strings.HasPrefix(p, "<h") || strings.HasPrefix(p, "<pre") ||
			strings.HasPrefix(p, "<li") || strings.HasPrefix(p, "<ul") ||
			strings.HasPrefix(p, "<ol") {
			result = append(result, p)
		} else {
			// Convert single newlines to <br>
			p = strings.ReplaceAll(p, "\n", "<br>\n")
			result = append(result, "<p>"+p+"</p>")
		}
	}

	return strings.Join(result, "\n")
}

// TruncateText truncates text to a maximum length
func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// EscapeHTML escapes HTML special characters
func EscapeHTML(text string) string {
	return html.EscapeString(text)
}
