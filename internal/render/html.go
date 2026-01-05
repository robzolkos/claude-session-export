package render

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/robzolkos/claude-session-export/internal/session"
)

// ErrNoConversations is returned when a session has no conversations to render
var ErrNoConversations = errors.New("no conversations found in session")

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
	Metadata       *session.SessionMetadata
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
		return ErrNoConversations
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
		Metadata:     g.Session.Metadata,
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

	// Build session metadata HTML
	metaHTML := g.renderSessionMetadata(data.Metadata)

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
	`, stats, metaHTML, pagination, itemsHTML.String(), pagination, g.getSearchJS(data.TotalPages))

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

func (g *Generator) renderSessionMetadata(meta *session.SessionMetadata) string {
	if meta == nil {
		return ""
	}

	var items []string

	// Models used
	if len(meta.Models) > 0 {
		var modelBadges []string
		for _, m := range meta.Models {
			modelBadges = append(modelBadges, fmt.Sprintf(`<span class="meta-value">%s</span>`, html.EscapeString(formatModelName(m))))
		}
		items = append(items, fmt.Sprintf(`<div class="meta-item"><span class="meta-label">Models</span>%s</div>`, strings.Join(modelBadges, " ")))
	}

	// Token totals
	if meta.TotalInput > 0 || meta.TotalOutput > 0 {
		tokenStr := fmt.Sprintf(`<span class="meta-value token-in">↓%s in</span> <span class="meta-value token-out">↑%s out</span>`,
			formatTokenCount(meta.TotalInput), formatTokenCount(meta.TotalOutput))
		if meta.TotalCache > 0 {
			tokenStr += fmt.Sprintf(` <span class="meta-value token-cache">⚡%s cache</span>`, formatTokenCount(meta.TotalCache))
		}
		items = append(items, fmt.Sprintf(`<div class="meta-item"><span class="meta-label">Tokens</span>%s</div>`, tokenStr))
	}

	// Working directory
	if meta.Cwd != "" {
		items = append(items, fmt.Sprintf(`<div class="meta-item"><span class="meta-label">Directory</span><span class="meta-value">%s</span></div>`, html.EscapeString(meta.Cwd)))
	}

	// Git branch
	if meta.GitBranch != "" {
		items = append(items, fmt.Sprintf(`<div class="meta-item"><span class="meta-label">Branch</span><span class="meta-value">%s</span></div>`, html.EscapeString(meta.GitBranch)))
	}

	// Claude Code version
	if meta.Version != "" {
		items = append(items, fmt.Sprintf(`<div class="meta-item"><span class="meta-label">Version</span><span class="meta-value">%s</span></div>`, html.EscapeString(meta.Version)))
	}

	if len(items) == 0 {
		return ""
	}

	return fmt.Sprintf(`<div class="session-meta">%s</div>`, strings.Join(items, ""))
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
@import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@300;400;500;600;700&display=swap');

:root {
	--bg-primary: #0a0a0f;
	--bg-secondary: #12121a;
	--bg-tertiary: #1a1a24;
	--bg-elevated: #22222e;

	--neon-pink: #ff0080;
	--neon-cyan: #00f0ff;
	--neon-green: #39ff14;
	--neon-orange: #ff6600;
	--neon-purple: #bf00ff;
	--neon-yellow: #ffee00;

	--text-primary: #e8e8ef;
	--text-secondary: #9898a8;
	--text-muted: #5c5c6e;

	--glow-pink: 0 0 20px rgba(255, 0, 128, 0.4);
	--glow-cyan: 0 0 20px rgba(0, 240, 255, 0.4);
	--glow-green: 0 0 20px rgba(57, 255, 20, 0.4);
	--glow-orange: 0 0 15px rgba(255, 102, 0, 0.3);

	--border-subtle: rgba(255, 255, 255, 0.06);
	--border-glow: rgba(0, 240, 255, 0.2);
}

* {
	box-sizing: border-box;
}

body {
	font-family: 'Outfit', -apple-system, BlinkMacSystemFont, sans-serif;
	font-weight: 400;
	line-height: 1.7;
	color: var(--text-primary);
	background: var(--bg-primary);
	margin: 0;
	padding: 24px;
	min-height: 100vh;
	background-image:
		radial-gradient(ellipse at 20% 0%, rgba(191, 0, 255, 0.08) 0%, transparent 50%),
		radial-gradient(ellipse at 80% 100%, rgba(0, 240, 255, 0.06) 0%, transparent 50%);
}

.container {
	max-width: 960px;
	margin: 0 auto;
}

h1 {
	font-weight: 600;
	font-size: 1.75rem;
	margin: 0 0 8px 0;
	background: linear-gradient(135deg, var(--neon-cyan) 0%, var(--neon-pink) 100%);
	-webkit-background-clip: text;
	-webkit-text-fill-color: transparent;
	background-clip: text;
	letter-spacing: -0.02em;
}

h1 a {
	background: linear-gradient(135deg, var(--neon-cyan) 0%, var(--neon-pink) 100%);
	-webkit-background-clip: text;
	-webkit-text-fill-color: transparent;
	background-clip: text;
	text-decoration: none;
}

.stats {
	color: var(--text-secondary);
	font-size: 0.875rem;
	margin-bottom: 24px;
	padding: 12px 16px;
	background: var(--bg-secondary);
	border-radius: 8px;
	border: 1px solid var(--border-subtle);
	display: inline-block;
}

.session-meta {
	display: flex;
	flex-wrap: wrap;
	gap: 16px;
	margin-bottom: 24px;
	padding: 16px;
	background: linear-gradient(135deg, rgba(191, 0, 255, 0.05) 0%, rgba(0, 240, 255, 0.05) 100%);
	border-radius: 12px;
	border: 1px solid var(--border-subtle);
}

.meta-item {
	display: flex;
	align-items: center;
	gap: 8px;
	font-size: 0.8rem;
	color: var(--text-secondary);
}

.meta-label {
	color: var(--text-muted);
	text-transform: uppercase;
	font-size: 0.65rem;
	letter-spacing: 0.1em;
}

.meta-value {
	font-family: 'JetBrains Mono', monospace;
	color: var(--neon-cyan);
}

.message {
	margin-bottom: 24px;
	border-radius: 12px;
	padding: 20px;
	border: 1px solid var(--border-subtle);
	position: relative;
	overflow: hidden;
}

.message::before {
	content: '';
	position: absolute;
	top: 0;
	left: 0;
	width: 3px;
	height: 100%;
}

.message.user {
	background: linear-gradient(135deg, rgba(0, 240, 255, 0.08) 0%, rgba(0, 240, 255, 0.02) 100%);
	border-color: rgba(0, 240, 255, 0.15);
}

.message.user::before {
	background: linear-gradient(180deg, var(--neon-cyan) 0%, rgba(0, 240, 255, 0.3) 100%);
	box-shadow: var(--glow-cyan);
}

.message.assistant {
	background: linear-gradient(135deg, rgba(255, 0, 128, 0.06) 0%, rgba(191, 0, 255, 0.03) 100%);
	border-color: rgba(255, 0, 128, 0.12);
}

.message.assistant::before {
	background: linear-gradient(180deg, var(--neon-pink) 0%, var(--neon-purple) 100%);
	box-shadow: var(--glow-pink);
}

.message-header {
	display: flex;
	align-items: center;
	gap: 12px;
	margin-bottom: 16px;
	flex-wrap: wrap;
}

.role-label {
	font-weight: 600;
	font-size: 0.8rem;
	text-transform: uppercase;
	letter-spacing: 0.08em;
}

.user .role-label {
	color: var(--neon-cyan);
}

.assistant .role-label {
	color: var(--neon-pink);
}

.model-badge {
	font-family: 'JetBrains Mono', monospace;
	font-size: 0.7rem;
	font-weight: 500;
	padding: 4px 10px;
	background: linear-gradient(135deg, rgba(191, 0, 255, 0.2) 0%, rgba(255, 0, 128, 0.2) 100%);
	border: 1px solid rgba(191, 0, 255, 0.3);
	border-radius: 100px;
	color: var(--neon-purple);
}

.timestamp {
	font-size: 0.75rem;
	color: var(--text-muted);
	margin-left: auto;
	font-family: 'JetBrains Mono', monospace;
}

.token-usage {
	display: flex;
	gap: 16px;
	margin-top: 16px;
	padding-top: 12px;
	border-top: 1px solid var(--border-subtle);
	font-family: 'JetBrains Mono', monospace;
	font-size: 0.7rem;
}

.token-in {
	color: var(--neon-cyan);
}

.token-out {
	color: var(--neon-pink);
}

.token-cache {
	color: var(--neon-yellow);
}

.text-block {
	margin-bottom: 12px;
}

.text-block p {
	margin: 0 0 12px 0;
}

.text-block p:last-child {
	margin-bottom: 0;
}

.thinking-block {
	background: linear-gradient(135deg, rgba(255, 238, 0, 0.08) 0%, rgba(255, 102, 0, 0.04) 100%);
	border: 1px solid rgba(255, 238, 0, 0.15);
	border-left: 3px solid var(--neon-yellow);
	padding: 16px;
	margin: 16px 0;
	border-radius: 8px;
}

.thinking-label {
	font-weight: 600;
	font-size: 0.75rem;
	text-transform: uppercase;
	letter-spacing: 0.1em;
	color: var(--neon-yellow);
	margin-bottom: 8px;
}

.tool-block {
	background: var(--bg-secondary);
	border: 1px solid var(--border-subtle);
	border-left: 3px solid var(--neon-orange);
	padding: 16px;
	margin: 16px 0;
	border-radius: 8px;
}

.tool-header {
	font-weight: 600;
	font-size: 0.85rem;
	margin-bottom: 12px;
	color: var(--neon-orange);
	display: flex;
	align-items: center;
	gap: 8px;
}

.tool-description {
	font-weight: 400;
	color: var(--text-secondary);
	font-size: 0.8rem;
}

pre {
	background: var(--bg-primary);
	border: 1px solid var(--border-subtle);
	color: var(--neon-green);
	padding: 16px;
	border-radius: 8px;
	overflow-x: auto;
	margin: 12px 0;
	font-size: 0.8rem;
	line-height: 1.5;
}

code {
	font-family: 'JetBrains Mono', monospace;
}

.edit-diff {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 12px;
}

@media (max-width: 640px) {
	.edit-diff {
		grid-template-columns: 1fr;
	}
}

.diff-old {
	border: 1px solid rgba(255, 0, 128, 0.3);
	border-radius: 8px;
	padding: 12px;
	background: rgba(255, 0, 128, 0.05);
}

.diff-new {
	border: 1px solid rgba(57, 255, 20, 0.3);
	border-radius: 8px;
	padding: 12px;
	background: rgba(57, 255, 20, 0.05);
}

.diff-label {
	font-size: 0.7rem;
	font-weight: 600;
	text-transform: uppercase;
	letter-spacing: 0.1em;
	margin-bottom: 8px;
}

.diff-old .diff-label {
	color: var(--neon-pink);
}

.diff-new .diff-label {
	color: var(--neon-green);
}

.tool-result {
	margin: 12px 0;
}

.tool-error {
	background: rgba(255, 0, 128, 0.08);
	border: 1px solid rgba(255, 0, 128, 0.2);
	border-left: 3px solid var(--neon-pink);
	padding: 12px;
	border-radius: 8px;
}

.commit-card {
	background: linear-gradient(135deg, rgba(57, 255, 20, 0.1) 0%, rgba(57, 255, 20, 0.02) 100%);
	border: 1px solid rgba(57, 255, 20, 0.2);
	padding: 12px 16px;
	border-radius: 8px;
	margin: 12px 0;
	display: inline-flex;
	align-items: center;
	gap: 10px;
}

.commit-hash {
	font-family: 'JetBrains Mono', monospace;
	color: var(--neon-green);
	font-weight: 500;
}

.commit-hash:hover {
	text-shadow: var(--glow-green);
}

.todo-list {
	list-style: none;
	padding: 0;
	margin: 0;
}

.todo-list li {
	padding: 10px 0;
	border-bottom: 1px solid var(--border-subtle);
	display: flex;
	align-items: center;
	gap: 10px;
}

.todo-list li:last-child {
	border-bottom: none;
}

.todo-status {
	font-size: 1rem;
}

.todo-completed {
	text-decoration: line-through;
	color: var(--text-muted);
}

.todo-completed .todo-status {
	color: var(--neon-green);
}

.todo-in_progress {
	color: var(--neon-orange);
}

.todo-in_progress .todo-status {
	color: var(--neon-orange);
}

.pagination {
	display: flex;
	justify-content: center;
	gap: 8px;
	margin: 32px 0;
	flex-wrap: wrap;
}

.page-link {
	padding: 10px 16px;
	border: 1px solid var(--border-subtle);
	border-radius: 8px;
	text-decoration: none;
	color: var(--text-secondary);
	font-family: 'JetBrains Mono', monospace;
	font-size: 0.8rem;
	transition: all 0.2s ease;
	background: var(--bg-secondary);
}

.page-link:hover {
	background: var(--bg-elevated);
	border-color: var(--neon-cyan);
	color: var(--neon-cyan);
	box-shadow: var(--glow-cyan);
}

.page-link.current {
	background: linear-gradient(135deg, var(--neon-cyan) 0%, var(--neon-pink) 100%);
	color: var(--bg-primary);
	border-color: transparent;
	font-weight: 600;
}

.page-link.disabled {
	color: var(--text-muted);
	cursor: not-allowed;
	opacity: 0.5;
}

.index-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	flex-wrap: wrap;
	gap: 16px;
	margin-bottom: 8px;
}

.search-box {
	display: flex;
	gap: 8px;
}

.search-box input {
	padding: 10px 16px;
	border: 1px solid var(--border-subtle);
	border-radius: 8px;
	font-size: 0.9rem;
	background: var(--bg-secondary);
	color: var(--text-primary);
	font-family: inherit;
	transition: all 0.2s ease;
}

.search-box input:focus {
	outline: none;
	border-color: var(--neon-cyan);
	box-shadow: var(--glow-cyan);
}

.search-box input::placeholder {
	color: var(--text-muted);
}

.search-box button {
	padding: 10px 16px;
	border: 1px solid var(--border-subtle);
	border-radius: 8px;
	background: var(--bg-elevated);
	color: var(--text-primary);
	cursor: pointer;
	transition: all 0.2s ease;
}

.search-box button:hover {
	border-color: var(--neon-pink);
	box-shadow: var(--glow-pink);
}

.index-items {
	margin: 24px 0;
}

.index-item {
	padding: 20px;
	border: 1px solid var(--border-subtle);
	border-radius: 12px;
	margin-bottom: 12px;
	background: var(--bg-secondary);
	transition: all 0.2s ease;
}

.index-item:hover {
	border-color: rgba(0, 240, 255, 0.3);
	background: var(--bg-tertiary);
	transform: translateX(4px);
}

.index-item-header {
	display: flex;
	justify-content: space-between;
	align-items: flex-start;
	gap: 16px;
}

.index-item-header a {
	color: var(--neon-cyan);
	text-decoration: none;
	flex: 1;
	font-weight: 500;
	line-height: 1.5;
}

.index-item-header a:hover {
	text-shadow: var(--glow-cyan);
}

.index-stats {
	font-size: 0.75rem;
	color: var(--text-muted);
	margin-top: 8px;
	font-family: 'JetBrains Mono', monospace;
}

.commit-item {
	background: linear-gradient(135deg, rgba(57, 255, 20, 0.08) 0%, rgba(57, 255, 20, 0.02) 100%);
	border-color: rgba(57, 255, 20, 0.15);
	display: flex;
	align-items: center;
	gap: 12px;
}

.commit-item:hover {
	border-color: rgba(57, 255, 20, 0.4);
}

.commit-icon {
	font-size: 1.25rem;
}

.commit-message {
	color: var(--text-secondary);
}

.truncated {
	color: var(--text-muted);
	font-style: italic;
}

dialog {
	border: none;
	border-radius: 16px;
	padding: 0;
	max-width: 90vw;
	width: 900px;
	max-height: 90vh;
	background: var(--bg-secondary);
	color: var(--text-primary);
	box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5), var(--glow-cyan);
}

dialog::backdrop {
	background: rgba(10, 10, 15, 0.9);
	backdrop-filter: blur(4px);
}

.search-dialog {
	padding: 24px;
}

.search-header {
	display: flex;
	gap: 12px;
	margin-bottom: 20px;
}

.search-header input {
	flex: 1;
	padding: 14px 18px;
	font-size: 1rem;
	border: 1px solid var(--border-subtle);
	border-radius: 10px;
	background: var(--bg-primary);
	color: var(--text-primary);
	font-family: inherit;
}

.search-header input:focus {
	outline: none;
	border-color: var(--neon-cyan);
}

.search-header button {
	padding: 14px 20px;
	border: 1px solid var(--border-subtle);
	border-radius: 10px;
	background: var(--bg-elevated);
	color: var(--text-primary);
	cursor: pointer;
	transition: all 0.2s ease;
}

.search-header button:hover {
	border-color: var(--neon-pink);
}

#search-status {
	color: var(--text-secondary);
	margin-bottom: 16px;
	font-size: 0.875rem;
}

#search-results {
	max-height: 60vh;
	overflow-y: auto;
}

.search-result {
	margin-bottom: 16px;
	border: 1px solid var(--border-subtle);
	border-radius: 12px;
	overflow: hidden;
}

.result-page {
	background: var(--bg-elevated);
	padding: 10px 16px;
	font-weight: 600;
	font-size: 0.8rem;
	color: var(--neon-cyan);
	border-bottom: 1px solid var(--border-subtle);
}

.image-block {
	margin: 16px 0;
}

.image-block img {
	max-width: 100%;
	border-radius: 8px;
	border: 1px solid var(--border-subtle);
}

a {
	color: var(--neon-cyan);
}

a:hover {
	text-shadow: var(--glow-cyan);
}

/* Scrollbar styling */
::-webkit-scrollbar {
	width: 8px;
	height: 8px;
}

::-webkit-scrollbar-track {
	background: var(--bg-secondary);
}

::-webkit-scrollbar-thumb {
	background: var(--bg-elevated);
	border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
	background: var(--text-muted);
}

/* Selection */
::selection {
	background: rgba(0, 240, 255, 0.3);
	color: var(--text-primary);
}
`
}
