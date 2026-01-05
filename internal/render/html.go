package render

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/robzolkos/claude-session-export/internal/session"
)

// Generator generates HTML output for a session
type Generator struct {
	Session     *session.Session
	OutputDir   string
	RepoURL     string
	IncludeJSON bool
	SourcePath  string
}

// PageData contains data for rendering a page
type PageData struct {
	PageNum    int
	TotalPages int
	Messages   []session.MessageEntry
}

// IndexData contains data for rendering the index
type IndexData struct {
	TotalPrompts   int
	TotalMessages  int
	TotalToolCalls int
	TotalCommits   int
	TotalPages     int
	Items          []session.IndexItem
}

// Generate generates all HTML files
func (g *Generator) Generate() error {
	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Detect GitHub repo if not provided
	if g.RepoURL == "" {
		g.RepoURL = session.DetectGitHubRepo(g.Session)
	}

	// Group into conversations
	conversations := session.GroupConversations(g.Session)
	if len(conversations) == 0 {
		return fmt.Errorf("no conversations found in session")
	}

	// Calculate pages
	totalPages := (len(conversations) + PromptsPerPage - 1) / PromptsPerPage

	// Collect index items and stats
	indexData := g.buildIndexData(conversations, totalPages)

	// Generate pages
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		start := (pageNum - 1) * PromptsPerPage
		end := start + PromptsPerPage
		if end > len(conversations) {
			end = len(conversations)
		}

		if err := g.generatePage(pageNum, totalPages, conversations[start:end], start); err != nil {
			return fmt.Errorf("generating page %d: %w", pageNum, err)
		}
	}

	// Generate index
	if err := g.generateIndex(indexData); err != nil {
		return fmt.Errorf("generating index: %w", err)
	}

	// Copy source file if requested
	if g.IncludeJSON && g.SourcePath != "" {
		if err := g.copySourceFile(); err != nil {
			return fmt.Errorf("copying source file: %w", err)
		}
	}

	return nil
}

func (g *Generator) buildIndexData(conversations []session.Conversation, totalPages int) *IndexData {
	data := &IndexData{
		TotalPrompts: len(conversations),
		TotalPages:   totalPages,
	}

	// Collect prompts with stats
	for i, conv := range conversations {
		pageNum := (i / PromptsPerPage) + 1
		stats, longTexts := session.AnalyzeConversation(&conv)

		data.TotalMessages += len(conv.Messages)
		data.TotalToolCalls += stats.BashCount + stats.ReadCount + stats.WriteCount +
			stats.EditCount + stats.GlobCount + stats.GrepCount + stats.OtherCount

		item := session.IndexItem{
			Type:      "prompt",
			Timestamp: conv.Timestamp,
			Text:      TruncateText(conv.UserText, 200),
			PageNum:   pageNum,
			MessageID: fmt.Sprintf("msg-%d", i),
			Stats:     stats,
			LongTexts: longTexts,
		}
		data.Items = append(data.Items, item)
	}

	// Extract commits
	commits := session.ExtractCommits(g.Session)
	for i := range commits {
		commits[i].RepoURL = g.RepoURL
	}
	data.TotalCommits = len(commits)
	data.Items = append(data.Items, commits...)

	// Sort by timestamp
	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Timestamp.Before(data.Items[j].Timestamp)
	})

	return data
}

func (g *Generator) generatePage(pageNum, totalPages int, conversations []session.Conversation, startConvIdx int) error {
	var buf bytes.Buffer

	opts := &RenderOptions{RepoURL: g.RepoURL}

	// Render messages with conversation anchors
	var messagesHTML bytes.Buffer
	for convOffset, conv := range conversations {
		convIdx := startConvIdx + convOffset
		for msgIdx, msg := range conv.Messages {
			// First message of each conversation gets the anchor ID
			anchorID := ""
			if msgIdx == 0 {
				anchorID = fmt.Sprintf("msg-%d", convIdx)
			}
			messagesHTML.WriteString(RenderMessageWithAnchor(msg, anchorID, opts))
		}
	}

	// Build page
	title := fmt.Sprintf("Claude Code transcript - page %d", pageNum)
	heading := fmt.Sprintf(`<a href="index.html" style="color: inherit; text-decoration: none;">Claude Code transcript</a> - page %d/%d`, pageNum, totalPages)
	pagination := g.renderPagination(pageNum, totalPages, false)

	buf.WriteString(g.wrapHTML(title, fmt.Sprintf(`
		<h1>%s</h1>
		%s
		%s
		%s
	`, heading, pagination, messagesHTML.String(), pagination)))

	filename := fmt.Sprintf("page-%03d.html", pageNum)
	return os.WriteFile(filepath.Join(g.OutputDir, filename), buf.Bytes(), 0644)
}

func (g *Generator) generateIndex(data *IndexData) error {
	var buf bytes.Buffer

	// Build index items HTML
	var itemsHTML bytes.Buffer
	for _, item := range data.Items {
		if item.Type == "prompt" {
			itemsHTML.WriteString(g.renderIndexPrompt(item))
		} else {
			itemsHTML.WriteString(g.renderIndexCommit(item))
		}
	}

	stats := fmt.Sprintf("%d prompts · %d messages · %d tool calls · %d commits · %d pages",
		data.TotalPrompts, data.TotalMessages, data.TotalToolCalls, data.TotalCommits, data.TotalPages)

	pagination := g.renderPagination(0, data.TotalPages, true)

	content := fmt.Sprintf(`
		<div class="index-header">
			<h1>Claude Code transcript</h1>
			<div class="search-box">
				<input type="text" id="search-input" placeholder="Search..." aria-label="Search transcripts">
				<button id="search-btn" aria-label="Search">🔍</button>
			</div>
		</div>
		<p class="stats">%s</p>
		%s
		<div class="index-items">%s</div>
		%s
		<dialog id="search-modal">
			<div class="search-dialog">
				<div class="search-header">
					<input type="text" id="modal-search-input" placeholder="Search..." aria-label="Search">
					<button id="modal-search-btn">🔍</button>
					<button id="close-modal">✕</button>
				</div>
				<div id="search-status"></div>
				<div id="search-results"></div>
			</div>
		</dialog>
		<script>%s</script>
	`, stats, pagination, itemsHTML.String(), pagination, g.getSearchJS(data.TotalPages))

	buf.WriteString(g.wrapHTML("Claude Code transcript", content))

	return os.WriteFile(filepath.Join(g.OutputDir, "index.html"), buf.Bytes(), 0644)
}

func (g *Generator) renderIndexPrompt(item session.IndexItem) string {
	var buf bytes.Buffer

	buf.WriteString(`<div class="index-item">`)
	buf.WriteString(fmt.Sprintf(`<div class="index-item-header">
		<a href="page-%03d.html#%s">%s</a>
		<span class="timestamp">%s</span>
	</div>`, item.PageNum, item.MessageID, html.EscapeString(item.Text), formatTimestamp(item.Timestamp)))

	// Stats
	if item.Stats != nil {
		var statParts []string
		if item.Stats.BashCount > 0 {
			statParts = append(statParts, fmt.Sprintf("bash: %d", item.Stats.BashCount))
		}
		if item.Stats.ReadCount > 0 {
			statParts = append(statParts, fmt.Sprintf("read: %d", item.Stats.ReadCount))
		}
		if item.Stats.WriteCount > 0 {
			statParts = append(statParts, fmt.Sprintf("write: %d", item.Stats.WriteCount))
		}
		if item.Stats.EditCount > 0 {
			statParts = append(statParts, fmt.Sprintf("edit: %d", item.Stats.EditCount))
		}
		if len(statParts) > 0 {
			buf.WriteString(fmt.Sprintf(`<div class="index-stats">%s</div>`, strings.Join(statParts, " · ")))
		}
	}

	buf.WriteString(`</div>`)
	return buf.String()
}

func (g *Generator) renderIndexCommit(item session.IndexItem) string {
	var buf bytes.Buffer

	buf.WriteString(`<div class="index-item commit-item">`)
	buf.WriteString(`<span class="commit-icon">📦</span>`)

	hashDisplay := item.CommitHash
	if len(hashDisplay) > 7 {
		hashDisplay = hashDisplay[:7]
	}

	if item.RepoURL != "" {
		commitURL := fmt.Sprintf("%s/commit/%s", item.RepoURL, item.CommitHash)
		buf.WriteString(fmt.Sprintf(`<a href="%s" class="commit-hash">%s</a>`, commitURL, hashDisplay))
	} else {
		buf.WriteString(fmt.Sprintf(`<span class="commit-hash">%s</span>`, hashDisplay))
	}

	buf.WriteString(fmt.Sprintf(` <span class="commit-message">%s</span>`, html.EscapeString(item.CommitMessage)))
	buf.WriteString(fmt.Sprintf(`<span class="timestamp">%s</span>`, formatTimestamp(item.Timestamp)))
	buf.WriteString(`</div>`)

	return buf.String()
}

func (g *Generator) renderPagination(currentPage, totalPages int, isIndex bool) string {
	if totalPages <= 1 {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString(`<div class="pagination">`)

	// Previous link
	if isIndex {
		buf.WriteString(`<span class="page-link disabled">« Prev</span>`)
	} else if currentPage > 1 {
		buf.WriteString(fmt.Sprintf(`<a href="page-%03d.html" class="page-link">« Prev</a>`, currentPage-1))
	} else {
		buf.WriteString(`<span class="page-link disabled">« Prev</span>`)
	}

	// Page numbers
	for p := 1; p <= totalPages; p++ {
		if isIndex {
			buf.WriteString(fmt.Sprintf(`<a href="page-%03d.html" class="page-link">%d</a>`, p, p))
		} else if p == currentPage {
			buf.WriteString(fmt.Sprintf(`<span class="page-link current">%d</span>`, p))
		} else {
			buf.WriteString(fmt.Sprintf(`<a href="page-%03d.html" class="page-link">%d</a>`, p, p))
		}
	}

	// Next link
	if isIndex {
		buf.WriteString(`<a href="page-001.html" class="page-link">Next »</a>`)
	} else if currentPage < totalPages {
		buf.WriteString(fmt.Sprintf(`<a href="page-%03d.html" class="page-link">Next »</a>`, currentPage+1))
	} else {
		buf.WriteString(`<span class="page-link disabled">Next »</span>`)
	}

	buf.WriteString(`</div>`)
	return buf.String()
}

func (g *Generator) wrapHTML(title, content string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<style>%s</style>
</head>
<body>
	<div class="container">
		%s
	</div>
	<script>%s</script>
</body>
</html>`, html.EscapeString(title), getCSS(), content, getGistPreviewJS())
}

func getGistPreviewJS() string {
	return `
// Fix navigation for gistpreview.github.io
(function() {
	if (!window.location.hostname.includes('gistpreview.github.io')) return;

	// Extract gist ID from URL (format: ?gist_id/filename or ?gist_id)
	var search = window.location.search.slice(1); // Remove leading ?
	var parts = search.split('/');
	var gistId = parts[0];
	if (!gistId) return;

	// Rewrite all relative links to use gistpreview format
	document.querySelectorAll('a[href]').forEach(function(a) {
		var href = a.getAttribute('href');
		// Skip external links and anchors
		if (href.startsWith('http') || href.startsWith('#') || href.startsWith('mailto:')) return;

		// Handle links like "page-001.html" or "page-001.html#msg-5" or "index.html"
		var hashIdx = href.indexOf('#');
		var filename = hashIdx >= 0 ? href.slice(0, hashIdx) : href;
		var hash = hashIdx >= 0 ? href.slice(hashIdx) : '';

		// Rewrite to gistpreview format
		a.href = '?' + gistId + '/' + filename + hash;
	});
})();
`
}

func (g *Generator) copySourceFile() error {
	data, err := os.ReadFile(g.SourcePath)
	if err != nil {
		return err
	}

	filename := filepath.Base(g.SourcePath)
	return os.WriteFile(filepath.Join(g.OutputDir, filename), data, 0644)
}

func (g *Generator) getSearchJS(totalPages int) string {
	return fmt.Sprintf(`
(function() {
	const totalPages = %d;
	const searchInput = document.getElementById('search-input');
	const searchBtn = document.getElementById('search-btn');
	const modal = document.getElementById('search-modal');
	const modalInput = document.getElementById('modal-search-input');
	const modalSearchBtn = document.getElementById('modal-search-btn');
	const closeModal = document.getElementById('close-modal');
	const searchStatus = document.getElementById('search-status');
	const searchResults = document.getElementById('search-results');

	function openModal(query) {
		modal.showModal();
		modalInput.value = query || '';
		if (query) doSearch(query);
	}

	searchBtn.addEventListener('click', () => openModal(searchInput.value));
	searchInput.addEventListener('keypress', (e) => {
		if (e.key === 'Enter') openModal(searchInput.value);
	});

	closeModal.addEventListener('click', () => modal.close());
	modalSearchBtn.addEventListener('click', () => doSearch(modalInput.value));
	modalInput.addEventListener('keypress', (e) => {
		if (e.key === 'Enter') doSearch(modalInput.value);
	});

	async function doSearch(query) {
		if (!query.trim()) return;

		searchStatus.textContent = 'Searching...';
		searchResults.innerHTML = '';

		const results = [];
		for (let i = 1; i <= totalPages; i++) {
			searchStatus.textContent = 'Searching page ' + i + ' of ' + totalPages + '...';
			try {
				const resp = await fetch('page-' + String(i).padStart(3, '0') + '.html');
				const html = await resp.text();
				const doc = new DOMParser().parseFromString(html, 'text/html');
				const messages = doc.querySelectorAll('.message');

				messages.forEach((msg) => {
					if (msg.textContent.toLowerCase().includes(query.toLowerCase())) {
						const clone = msg.cloneNode(true);
						clone.querySelectorAll('a').forEach((a) => {
							if (a.href.startsWith('#')) {
								a.href = 'page-' + String(i).padStart(3, '0') + '.html' + a.getAttribute('href');
							}
						});
						results.push({page: i, html: clone.outerHTML});
					}
				});
			} catch (e) {
				console.error('Error searching page', i, e);
			}
		}

		searchStatus.textContent = 'Found ' + results.length + ' results';
		results.forEach((r) => {
			const div = document.createElement('div');
			div.className = 'search-result';
			div.innerHTML = '<div class="result-page">Page ' + r.page + '</div>' + r.html;
			searchResults.appendChild(div);
		});

		// Update URL hash
		location.hash = 'search=' + encodeURIComponent(query);
	}

	// Check for search in hash on load
	if (location.hash.startsWith('#search=')) {
		const query = decodeURIComponent(location.hash.slice(8));
		openModal(query);
	}
})();
`, totalPages)
}

func getCSS() string {
	return `
:root {
	--bg-color: #ffffff;
	--text-color: #333333;
	--user-bg: #e3f2fd;
	--user-border: #1976d2;
	--assistant-bg: #f5f5f5;
	--assistant-border: #9e9e9e;
	--thinking-bg: #fff8e1;
	--thinking-border: #ffc107;
	--tool-bg: #f3e5f5;
	--tool-border: #9c27b0;
	--code-bg: #263238;
	--code-color: #aed581;
	--commit-bg: #e8f5e9;
	--commit-border: #4caf50;
	--error-bg: #ffebee;
	--error-border: #f44336;
	--text-muted: #757575;
}

* {
	box-sizing: border-box;
}

body {
	font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
	line-height: 1.6;
	color: var(--text-color);
	background: var(--bg-color);
	margin: 0;
	padding: 20px;
}

.container {
	max-width: 900px;
	margin: 0 auto;
}

h1 {
	margin-bottom: 10px;
}

h1 a {
	color: inherit;
	text-decoration: none;
}

.stats {
	color: var(--text-muted);
	margin-bottom: 20px;
}

.message {
	margin-bottom: 20px;
	border-radius: 8px;
	padding: 15px;
	border-left: 4px solid;
}

.message.user {
	background: var(--user-bg);
	border-color: var(--user-border);
}

.message.assistant {
	background: var(--assistant-bg);
	border-color: var(--assistant-border);
}

.message-header {
	font-weight: bold;
	margin-bottom: 10px;
	display: flex;
	justify-content: space-between;
	align-items: center;
}

.timestamp {
	font-size: 0.85em;
	color: var(--text-muted);
	font-weight: normal;
}

.text-block {
	margin-bottom: 10px;
}

.text-block p {
	margin: 0 0 10px 0;
}

.thinking-block {
	background: var(--thinking-bg);
	border-left: 3px solid var(--thinking-border);
	padding: 10px;
	margin: 10px 0;
	border-radius: 4px;
}

.thinking-label {
	font-weight: bold;
	color: var(--thinking-border);
	margin-bottom: 5px;
}

.tool-block {
	background: var(--tool-bg);
	border-left: 3px solid var(--tool-border);
	padding: 10px;
	margin: 10px 0;
	border-radius: 4px;
}

.tool-header {
	font-weight: bold;
	margin-bottom: 8px;
}

.tool-description {
	font-weight: normal;
	color: var(--text-muted);
}

pre {
	background: var(--code-bg);
	color: var(--code-color);
	padding: 12px;
	border-radius: 4px;
	overflow-x: auto;
	margin: 8px 0;
	font-size: 0.9em;
}

code {
	font-family: 'SF Mono', Monaco, 'Courier New', monospace;
}

.edit-diff {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 10px;
}

@media (max-width: 600px) {
	.edit-diff {
		grid-template-columns: 1fr;
	}
}

.diff-old {
	border: 1px solid #f44336;
	border-radius: 4px;
	padding: 8px;
}

.diff-new {
	border: 1px solid #4caf50;
	border-radius: 4px;
	padding: 8px;
}

.diff-label {
	font-size: 0.8em;
	font-weight: bold;
	margin-bottom: 5px;
}

.diff-old .diff-label {
	color: #f44336;
}

.diff-new .diff-label {
	color: #4caf50;
}

.tool-result {
	margin: 10px 0;
}

.tool-error {
	background: var(--error-bg);
	border-left: 3px solid var(--error-border);
	padding: 10px;
	border-radius: 4px;
}

.commit-card {
	background: var(--commit-bg);
	border: 1px solid var(--commit-border);
	padding: 10px;
	border-radius: 4px;
	margin: 10px 0;
	display: inline-block;
}

.commit-hash {
	font-family: monospace;
	color: var(--commit-border);
}

.todo-list {
	list-style: none;
	padding: 0;
	margin: 0;
}

.todo-list li {
	padding: 5px 0;
	border-bottom: 1px solid #eee;
}

.todo-list li:last-child {
	border-bottom: none;
}

.todo-status {
	margin-right: 8px;
}

.todo-completed {
	text-decoration: line-through;
	color: var(--text-muted);
}

.todo-in_progress {
	color: #ff9800;
}

.pagination {
	display: flex;
	justify-content: center;
	gap: 5px;
	margin: 20px 0;
	flex-wrap: wrap;
}

.page-link {
	padding: 8px 12px;
	border: 1px solid #ddd;
	border-radius: 4px;
	text-decoration: none;
	color: var(--text-color);
}

.page-link:hover {
	background: #f0f0f0;
}

.page-link.current {
	background: var(--user-border);
	color: white;
	border-color: var(--user-border);
}

.page-link.disabled {
	color: var(--text-muted);
	cursor: not-allowed;
}

.index-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	flex-wrap: wrap;
	gap: 10px;
}

.search-box {
	display: flex;
	gap: 5px;
}

.search-box input {
	padding: 8px 12px;
	border: 1px solid #ddd;
	border-radius: 4px;
	font-size: 1em;
}

.search-box button {
	padding: 8px 12px;
	border: 1px solid #ddd;
	border-radius: 4px;
	background: #f5f5f5;
	cursor: pointer;
}

.index-items {
	margin: 20px 0;
}

.index-item {
	padding: 15px;
	border: 1px solid #ddd;
	border-radius: 8px;
	margin-bottom: 10px;
}

.index-item:hover {
	background: #f9f9f9;
}

.index-item-header {
	display: flex;
	justify-content: space-between;
	align-items: flex-start;
	gap: 10px;
}

.index-item-header a {
	color: var(--user-border);
	text-decoration: none;
	flex: 1;
}

.index-item-header a:hover {
	text-decoration: underline;
}

.index-stats {
	font-size: 0.85em;
	color: var(--text-muted);
	margin-top: 5px;
}

.commit-item {
	background: var(--commit-bg);
	border-color: var(--commit-border);
	display: flex;
	align-items: center;
	gap: 10px;
}

.commit-icon {
	font-size: 1.2em;
}

.truncated {
	color: var(--text-muted);
	font-style: italic;
}

dialog {
	border: none;
	border-radius: 8px;
	padding: 0;
	max-width: 90vw;
	width: 800px;
	max-height: 90vh;
}

dialog::backdrop {
	background: rgba(0, 0, 0, 0.5);
}

.search-dialog {
	padding: 20px;
}

.search-header {
	display: flex;
	gap: 10px;
	margin-bottom: 15px;
}

.search-header input {
	flex: 1;
	padding: 10px;
	font-size: 1em;
	border: 1px solid #ddd;
	border-radius: 4px;
}

.search-header button {
	padding: 10px 15px;
	border: 1px solid #ddd;
	border-radius: 4px;
	background: #f5f5f5;
	cursor: pointer;
}

#search-status {
	color: var(--text-muted);
	margin-bottom: 15px;
}

#search-results {
	max-height: 60vh;
	overflow-y: auto;
}

.search-result {
	margin-bottom: 15px;
	border: 1px solid #ddd;
	border-radius: 8px;
	overflow: hidden;
}

.result-page {
	background: #f5f5f5;
	padding: 8px 15px;
	font-weight: bold;
	border-bottom: 1px solid #ddd;
}

.image-block {
	margin: 10px 0;
}

.image-block img {
	max-width: 100%;
	border-radius: 4px;
}
`
}
