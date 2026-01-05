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
	"time"

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
			// Skip tool-result-only entries (no actual user text)
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
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

	// Session duration
	if !meta.StartTime.IsZero() && !meta.EndTime.IsZero() {
		durationStr := formatDuration(meta.ActiveTime)
		// Show total span if significantly different from active time
		totalSpan := meta.EndTime.Sub(meta.StartTime)
		if totalSpan > meta.ActiveTime+(5*time.Minute) {
			durationStr += fmt.Sprintf(` <span class="meta-muted">(%s total)</span>`, formatDuration(totalSpan))
		}
		items = append(items, fmt.Sprintf(`<div class="meta-item"><span class="meta-label">Duration</span><span class="meta-value">%s</span></div>`, durationStr))
	}

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
@import url('https://fonts.googleapis.com/css2?family=Orbitron:wght@400;500;600;700;800;900&family=IBM+Plex+Mono:wght@400;500;600&display=swap');

:root {
	/* Synthwave 84 Sunset Palette */
	--sunset-top: #2d1b4e;
	--sunset-mid: #1a1a2e;
	--sunset-bottom: #0f0f23;
	--horizon-glow: #ff6b35;

	/* Neon Colors */
	--neon-pink: #ff2975;
	--neon-magenta: #f222ff;
	--neon-cyan: #00fff9;
	--neon-blue: #34d8eb;
	--neon-orange: #ff8c00;
	--neon-yellow: #fdfd96;
	--neon-green: #72f1b8;

	/* Chrome/Metal */
	--chrome-light: #fef5f1;
	--chrome-mid: #c4a7e7;
	--chrome-dark: #6e5494;

	/* Backgrounds */
	--bg-primary: #0f0f1a;
	--bg-secondary: #16162a;
	--bg-tertiary: #1e1e3f;
	--bg-elevated: #2a2a4a;

	/* Text */
	--text-primary: #eeeef0;
	--text-secondary: #b4b4c4;
	--text-muted: #6c6c8a;

	/* Glows */
	--glow-pink: 0 0 20px rgba(255, 41, 117, 0.6), 0 0 40px rgba(255, 41, 117, 0.3);
	--glow-cyan: 0 0 20px rgba(0, 255, 249, 0.6), 0 0 40px rgba(0, 255, 249, 0.3);
	--glow-orange: 0 0 15px rgba(255, 140, 0, 0.5);
	--glow-magenta: 0 0 30px rgba(242, 34, 255, 0.5);

	/* Grid */
	--grid-color: rgba(242, 34, 255, 0.15);
	--grid-bright: rgba(242, 34, 255, 0.4);

	--border-subtle: rgba(242, 34, 255, 0.12);
	--border-glow: rgba(255, 41, 117, 0.3);
}

* {
	box-sizing: border-box;
}

/* Scanline overlay */
@keyframes scanline {
	0% { transform: translateY(-100%); }
	100% { transform: translateY(100vh); }
}

@keyframes flicker {
	0%, 100% { opacity: 1; }
	92% { opacity: 1; }
	93% { opacity: 0.8; }
	94% { opacity: 1; }
	97% { opacity: 0.9; }
}

@keyframes glow-pulse {
	0%, 100% { filter: brightness(1); }
	50% { filter: brightness(1.2); }
}

body {
	font-family: 'IBM Plex Mono', monospace;
	font-weight: 400;
	line-height: 1.7;
	color: var(--text-primary);
	margin: 0;
	padding: 0;
	min-height: 100vh;
	background: var(--bg-primary);
	position: relative;
	overflow-x: hidden;
}

/* Dark background with subtle purple hints */
body::before {
	content: '';
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background:
		/* Stars */
		radial-gradient(1px 1px at 20% 10%, rgba(255,255,255,0.4), transparent),
		radial-gradient(1px 1px at 40% 20%, rgba(255,255,255,0.3), transparent),
		radial-gradient(1px 1px at 60% 15%, rgba(255,255,255,0.4), transparent),
		radial-gradient(1px 1px at 80% 25%, rgba(255,255,255,0.2), transparent),
		radial-gradient(1.5px 1.5px at 10% 30%, rgba(255,255,255,0.3), transparent),
		radial-gradient(1px 1px at 90% 5%, rgba(255,255,255,0.3), transparent),
		/* Dark gradient with subtle purple glow */
		linear-gradient(180deg,
			#0f0f1a 0%,
			#12122a 50%,
			#0f0f1a 100%
		),
		/* Subtle corner glows */
		radial-gradient(ellipse at 0% 0%, rgba(242, 34, 255, 0.08) 0%, transparent 50%),
		radial-gradient(ellipse at 100% 100%, rgba(255, 41, 117, 0.06) 0%, transparent 50%);
	z-index: -2;
}

/* Perspective grid floor */
body::after {
	content: '';
	position: fixed;
	bottom: 0;
	left: -50%;
	right: -50%;
	height: 40vh;
	background:
		repeating-linear-gradient(
			90deg,
			transparent,
			transparent 60px,
			var(--grid-color) 60px,
			var(--grid-bright) 62px,
			var(--grid-color) 64px,
			transparent 64px
		),
		repeating-linear-gradient(
			0deg,
			transparent,
			transparent 30px,
			var(--grid-color) 30px,
			var(--grid-bright) 31px,
			var(--grid-color) 32px,
			transparent 32px
		);
	transform: perspective(500px) rotateX(60deg);
	transform-origin: center top;
	z-index: -1;
	mask-image: linear-gradient(to bottom, transparent 0%, black 20%, black 80%, transparent 100%);
	-webkit-mask-image: linear-gradient(to bottom, transparent 0%, black 20%, black 80%, transparent 100%);
}

/* CRT scanlines overlay */
.container::before {
	content: '';
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background: repeating-linear-gradient(
		0deg,
		transparent,
		transparent 2px,
		rgba(0, 0, 0, 0.1) 2px,
		rgba(0, 0, 0, 0.1) 4px
	);
	pointer-events: none;
	z-index: 1000;
}

.container {
	max-width: 960px;
	margin: 0 auto;
	padding: 24px;
	position: relative;
}

/* Chrome text effect for headings */
h1 {
	font-family: 'Orbitron', sans-serif;
	font-weight: 800;
	font-size: 2rem;
	margin: 0 0 16px 0;
	letter-spacing: 0.1em;
	text-transform: uppercase;
	background: linear-gradient(
		180deg,
		#fff 0%,
		#fff 35%,
		#c4a7e7 50%,
		#6e5494 65%,
		#2d1b4e 100%
	);
	-webkit-background-clip: text;
	-webkit-text-fill-color: transparent;
	background-clip: text;
	filter: drop-shadow(0 0 10px rgba(255, 41, 117, 0.5)) drop-shadow(0 2px 0 rgba(0,0,0,0.3));
	animation: glow-pulse 4s infinite;
}

h1 a {
	background: linear-gradient(
		180deg,
		#fff 0%,
		#fff 35%,
		#c4a7e7 50%,
		#6e5494 65%,
		#2d1b4e 100%
	);
	-webkit-background-clip: text;
	-webkit-text-fill-color: transparent;
	background-clip: text;
	text-decoration: none;
}

h2, h3, h4 {
	font-family: 'Orbitron', sans-serif;
	color: var(--neon-cyan);
	text-shadow: var(--glow-cyan);
	letter-spacing: 0.05em;
}

.stats {
	color: var(--neon-cyan);
	font-size: 0.875rem;
	margin-bottom: 24px;
	padding: 14px 20px;
	background: linear-gradient(135deg, rgba(242, 34, 255, 0.1) 0%, rgba(255, 41, 117, 0.05) 100%);
	border-radius: 4px;
	border: 1px solid var(--border-subtle);
	display: inline-block;
	font-family: 'IBM Plex Mono', monospace;
	text-shadow: 0 0 10px rgba(0, 255, 249, 0.5);
	box-shadow: inset 0 0 20px rgba(242, 34, 255, 0.1);
}

.session-meta {
	display: flex;
	flex-wrap: wrap;
	gap: 20px;
	margin-bottom: 28px;
	padding: 20px;
	background: linear-gradient(135deg, rgba(45, 27, 78, 0.6) 0%, rgba(15, 15, 35, 0.8) 100%);
	border-radius: 4px;
	border: 1px solid var(--grid-bright);
	box-shadow:
		inset 0 0 30px rgba(242, 34, 255, 0.1),
		0 0 20px rgba(242, 34, 255, 0.2);
}

.meta-item {
	display: flex;
	align-items: center;
	gap: 10px;
	font-size: 0.8rem;
	color: var(--text-secondary);
}

.meta-label {
	font-family: 'Orbitron', sans-serif;
	color: var(--neon-magenta);
	text-transform: uppercase;
	font-size: 0.6rem;
	font-weight: 600;
	letter-spacing: 0.15em;
	text-shadow: 0 0 8px rgba(242, 34, 255, 0.6);
}

.meta-value {
	font-family: 'IBM Plex Mono', monospace;
	color: var(--neon-cyan);
	text-shadow: 0 0 8px rgba(0, 255, 249, 0.5);
}

.meta-muted {
	color: var(--text-muted);
	text-shadow: none;
	font-size: 0.75rem;
}

.message {
	margin-bottom: 28px;
	border-radius: 4px;
	padding: 24px;
	border: 1px solid var(--border-subtle);
	position: relative;
	overflow: hidden;
	backdrop-filter: blur(10px);
}

.message::before {
	content: '';
	position: absolute;
	top: 0;
	left: 0;
	width: 4px;
	height: 100%;
}

/* Glowing corner accents */
.message::after {
	content: '';
	position: absolute;
	top: 0;
	right: 0;
	width: 40px;
	height: 40px;
	border-top: 2px solid;
	border-right: 2px solid;
	border-color: inherit;
	opacity: 0.5;
}

.message.user {
	background: linear-gradient(135deg, rgba(0, 255, 249, 0.08) 0%, rgba(52, 216, 235, 0.03) 100%);
	border-color: rgba(0, 255, 249, 0.2);
}

.message.user::before {
	background: linear-gradient(180deg, var(--neon-cyan) 0%, var(--neon-blue) 100%);
	box-shadow: var(--glow-cyan);
}

.message.user::after {
	border-color: var(--neon-cyan);
}

.message.assistant {
	background: linear-gradient(135deg, rgba(255, 41, 117, 0.08) 0%, rgba(242, 34, 255, 0.04) 100%);
	border-color: rgba(255, 41, 117, 0.2);
}

.message.assistant::before {
	background: linear-gradient(180deg, var(--neon-pink) 0%, var(--neon-magenta) 100%);
	box-shadow: var(--glow-pink);
}

.message.assistant::after {
	border-color: var(--neon-pink);
}

.message-header {
	display: flex;
	align-items: center;
	gap: 14px;
	margin-bottom: 18px;
	flex-wrap: wrap;
}

.role-label {
	font-family: 'Orbitron', sans-serif;
	font-weight: 700;
	font-size: 0.75rem;
	text-transform: uppercase;
	letter-spacing: 0.15em;
}

.user .role-label {
	color: var(--neon-cyan);
	text-shadow: var(--glow-cyan);
}

.assistant .role-label {
	color: var(--neon-pink);
	text-shadow: var(--glow-pink);
}

.role-emoji, .block-emoji {
	margin-right: 6px;
	filter: drop-shadow(0 0 4px currentColor);
}

.model-badge {
	font-family: 'Orbitron', sans-serif;
	font-size: 0.65rem;
	font-weight: 600;
	padding: 5px 12px;
	background: linear-gradient(135deg, rgba(242, 34, 255, 0.25) 0%, rgba(255, 41, 117, 0.15) 100%);
	border: 1px solid var(--neon-magenta);
	border-radius: 2px;
	color: var(--neon-magenta);
	text-transform: uppercase;
	letter-spacing: 0.1em;
	text-shadow: 0 0 10px rgba(242, 34, 255, 0.8);
	box-shadow: var(--glow-magenta);
}

.timestamp {
	font-size: 0.7rem;
	color: var(--text-muted);
	margin-left: auto;
	font-family: 'IBM Plex Mono', monospace;
	letter-spacing: 0.05em;
}

.token-usage {
	display: flex;
	gap: 20px;
	margin-top: 18px;
	padding-top: 14px;
	border-top: 1px solid var(--border-subtle);
	font-family: 'IBM Plex Mono', monospace;
	font-size: 0.7rem;
}

.token-in {
	color: var(--neon-cyan);
	text-shadow: 0 0 8px rgba(0, 255, 249, 0.6);
}

.token-out {
	color: var(--neon-pink);
	text-shadow: 0 0 8px rgba(255, 41, 117, 0.6);
}

.token-cache {
	color: var(--neon-yellow);
	text-shadow: 0 0 8px rgba(253, 253, 150, 0.6);
}

.text-block {
	margin-bottom: 14px;
}

.text-block p {
	margin: 0 0 14px 0;
}

.text-block p:last-child {
	margin-bottom: 0;
}

.thinking-block {
	background: linear-gradient(135deg, rgba(253, 253, 150, 0.1) 0%, rgba(255, 140, 0, 0.05) 100%);
	border: 1px solid rgba(253, 253, 150, 0.25);
	border-left: 4px solid var(--neon-yellow);
	padding: 18px;
	margin: 18px 0;
	border-radius: 4px;
	box-shadow: inset 0 0 20px rgba(253, 253, 150, 0.05);
}

.thinking-label {
	font-family: 'Orbitron', sans-serif;
	font-weight: 600;
	font-size: 0.7rem;
	text-transform: uppercase;
	letter-spacing: 0.15em;
	color: var(--neon-yellow);
	text-shadow: 0 0 10px rgba(253, 253, 150, 0.7);
	margin-bottom: 10px;
}

.tool-block {
	background: linear-gradient(135deg, rgba(255, 140, 0, 0.1) 0%, rgba(255, 107, 53, 0.05) 100%);
	border: 1px solid rgba(255, 140, 0, 0.25);
	border-left: 4px solid var(--neon-orange);
	padding: 18px;
	margin: 18px 0;
	border-radius: 4px;
	box-shadow: inset 0 0 20px rgba(255, 140, 0, 0.05);
}

.tool-header {
	font-family: 'Orbitron', sans-serif;
	font-weight: 600;
	font-size: 0.8rem;
	margin-bottom: 14px;
	color: var(--neon-orange);
	text-shadow: var(--glow-orange);
	display: flex;
	align-items: center;
	gap: 10px;
	text-transform: uppercase;
	letter-spacing: 0.08em;
}

.tool-description {
	font-family: 'IBM Plex Mono', monospace;
	font-weight: 400;
	color: var(--text-secondary);
	font-size: 0.75rem;
	text-transform: none;
	letter-spacing: normal;
}

pre {
	background: rgba(15, 15, 26, 0.9);
	border: 1px solid var(--grid-color);
	color: var(--neon-green);
	padding: 18px;
	border-radius: 4px;
	overflow-x: auto;
	margin: 14px 0;
	font-size: 0.8rem;
	line-height: 1.6;
	text-shadow: 0 0 10px rgba(114, 241, 184, 0.4);
	box-shadow: inset 0 0 30px rgba(0, 0, 0, 0.5);
}

code {
	font-family: 'IBM Plex Mono', monospace;
}

.edit-diff {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 14px;
}

@media (max-width: 640px) {
	.edit-diff {
		grid-template-columns: 1fr;
	}
}

.diff-old {
	border: 1px solid rgba(255, 41, 117, 0.4);
	border-radius: 4px;
	padding: 14px;
	background: rgba(255, 41, 117, 0.08);
}

.diff-new {
	border: 1px solid rgba(114, 241, 184, 0.4);
	border-radius: 4px;
	padding: 14px;
	background: rgba(114, 241, 184, 0.08);
}

.diff-label {
	font-family: 'Orbitron', sans-serif;
	font-size: 0.65rem;
	font-weight: 600;
	text-transform: uppercase;
	letter-spacing: 0.15em;
	margin-bottom: 10px;
}

.diff-old .diff-label {
	color: var(--neon-pink);
	text-shadow: 0 0 8px rgba(255, 41, 117, 0.6);
}

.diff-new .diff-label {
	color: var(--neon-green);
	text-shadow: 0 0 8px rgba(114, 241, 184, 0.6);
}

.tool-result {
	margin: 14px 0;
}

.tool-error {
	background: rgba(255, 41, 117, 0.1);
	border: 1px solid rgba(255, 41, 117, 0.3);
	border-left: 4px solid var(--neon-pink);
	padding: 14px;
	border-radius: 4px;
}

.commit-card {
	background: linear-gradient(135deg, rgba(114, 241, 184, 0.12) 0%, rgba(114, 241, 184, 0.04) 100%);
	border: 1px solid rgba(114, 241, 184, 0.3);
	padding: 14px 18px;
	border-radius: 4px;
	margin: 14px 0;
	display: inline-flex;
	align-items: center;
	gap: 12px;
	box-shadow: 0 0 15px rgba(114, 241, 184, 0.15);
}

.commit-hash {
	font-family: 'IBM Plex Mono', monospace;
	color: var(--neon-green);
	font-weight: 600;
	text-shadow: 0 0 8px rgba(114, 241, 184, 0.6);
}

.commit-hash:hover {
	text-shadow: 0 0 15px rgba(114, 241, 184, 0.9);
}

.todo-list {
	list-style: none;
	padding: 0;
	margin: 0;
}

.todo-list li {
	padding: 12px 0;
	border-bottom: 1px solid var(--border-subtle);
	display: flex;
	align-items: center;
	gap: 12px;
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
	text-shadow: 0 0 8px rgba(114, 241, 184, 0.6);
}

.todo-in_progress {
	color: var(--neon-orange);
}

.todo-in_progress .todo-status {
	color: var(--neon-orange);
	text-shadow: var(--glow-orange);
}

.pagination {
	display: flex;
	justify-content: center;
	gap: 10px;
	margin: 36px 0;
	flex-wrap: wrap;
}

.page-link {
	padding: 12px 18px;
	border: 1px solid var(--grid-bright);
	border-radius: 2px;
	text-decoration: none;
	color: var(--text-secondary);
	font-family: 'Orbitron', sans-serif;
	font-size: 0.75rem;
	font-weight: 500;
	letter-spacing: 0.05em;
	transition: all 0.3s ease;
	background: rgba(45, 27, 78, 0.4);
}

.page-link:hover {
	background: rgba(242, 34, 255, 0.2);
	border-color: var(--neon-magenta);
	color: var(--neon-magenta);
	box-shadow: var(--glow-magenta);
	text-shadow: 0 0 10px rgba(242, 34, 255, 0.8);
}

.page-link.current {
	background: linear-gradient(135deg, var(--neon-pink) 0%, var(--neon-magenta) 100%);
	color: #fff;
	border-color: transparent;
	font-weight: 700;
	box-shadow: var(--glow-pink);
	text-shadow: none;
}

.page-link.disabled {
	color: var(--text-muted);
	cursor: not-allowed;
	opacity: 0.4;
}

.index-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	flex-wrap: wrap;
	gap: 20px;
	margin-bottom: 12px;
}

.search-box {
	display: flex;
	gap: 10px;
}

.search-box input {
	padding: 12px 18px;
	border: 1px solid var(--grid-bright);
	border-radius: 2px;
	font-size: 0.9rem;
	background: rgba(22, 22, 42, 0.8);
	color: var(--text-primary);
	font-family: 'IBM Plex Mono', monospace;
	transition: all 0.3s ease;
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
	padding: 12px 18px;
	border: 1px solid var(--grid-bright);
	border-radius: 2px;
	background: rgba(45, 27, 78, 0.6);
	color: var(--text-primary);
	cursor: pointer;
	transition: all 0.3s ease;
}

.search-box button:hover {
	border-color: var(--neon-pink);
	box-shadow: var(--glow-pink);
}

.index-items {
	margin: 28px 0;
}

.index-item {
	padding: 22px;
	border: 1px solid var(--border-subtle);
	border-radius: 4px;
	margin-bottom: 14px;
	background: linear-gradient(135deg, rgba(45, 27, 78, 0.4) 0%, rgba(22, 22, 42, 0.6) 100%);
	transition: all 0.3s ease;
	position: relative;
}

.index-item::before {
	content: '';
	position: absolute;
	bottom: 0;
	left: 0;
	right: 0;
	height: 2px;
	background: linear-gradient(90deg, transparent, var(--neon-magenta), transparent);
	opacity: 0;
	transition: opacity 0.3s ease;
}

.index-item:hover {
	border-color: rgba(242, 34, 255, 0.4);
	background: linear-gradient(135deg, rgba(45, 27, 78, 0.6) 0%, rgba(30, 30, 63, 0.7) 100%);
	transform: translateX(6px);
	box-shadow: 0 0 25px rgba(242, 34, 255, 0.2);
}

.index-item:hover::before {
	opacity: 1;
}

.index-item-header {
	display: flex;
	justify-content: space-between;
	align-items: flex-start;
	gap: 18px;
}

.index-item-header a {
	color: var(--neon-cyan);
	text-decoration: none;
	flex: 1;
	font-weight: 500;
	line-height: 1.6;
	text-shadow: 0 0 8px rgba(0, 255, 249, 0.4);
	transition: all 0.3s ease;
}

.index-item-header a:hover {
	text-shadow: var(--glow-cyan);
}

.index-stats {
	font-size: 0.7rem;
	color: var(--text-muted);
	margin-top: 10px;
	font-family: 'IBM Plex Mono', monospace;
	letter-spacing: 0.03em;
}

.commit-item {
	background: linear-gradient(135deg, rgba(114, 241, 184, 0.1) 0%, rgba(114, 241, 184, 0.03) 100%);
	border-color: rgba(114, 241, 184, 0.2);
	display: flex;
	align-items: center;
	gap: 14px;
}

.commit-item:hover {
	border-color: rgba(114, 241, 184, 0.5);
	box-shadow: 0 0 25px rgba(114, 241, 184, 0.2);
}

.commit-icon {
	font-size: 1.3rem;
	filter: drop-shadow(0 0 5px rgba(114, 241, 184, 0.5));
}

.commit-message {
	color: var(--text-secondary);
}

.truncated {
	color: var(--text-muted);
	font-style: italic;
}

dialog {
	border: 2px solid var(--neon-magenta);
	border-radius: 4px;
	padding: 0;
	max-width: 90vw;
	width: 900px;
	max-height: 90vh;
	background: linear-gradient(180deg, rgba(45, 27, 78, 0.95) 0%, rgba(15, 15, 35, 0.98) 100%);
	color: var(--text-primary);
	box-shadow: 0 0 50px rgba(242, 34, 255, 0.4), inset 0 0 30px rgba(242, 34, 255, 0.1);
}

dialog::backdrop {
	background: rgba(15, 15, 26, 0.95);
	backdrop-filter: blur(8px);
}

.search-dialog {
	padding: 28px;
}

.search-header {
	display: flex;
	gap: 14px;
	margin-bottom: 24px;
}

.search-header input {
	flex: 1;
	padding: 16px 20px;
	font-size: 1rem;
	border: 1px solid var(--grid-bright);
	border-radius: 2px;
	background: rgba(15, 15, 26, 0.9);
	color: var(--text-primary);
	font-family: 'IBM Plex Mono', monospace;
}

.search-header input:focus {
	outline: none;
	border-color: var(--neon-cyan);
	box-shadow: var(--glow-cyan);
}

.search-header button {
	padding: 16px 22px;
	border: 1px solid var(--grid-bright);
	border-radius: 2px;
	background: rgba(45, 27, 78, 0.6);
	color: var(--text-primary);
	cursor: pointer;
	transition: all 0.3s ease;
}

.search-header button:hover {
	border-color: var(--neon-pink);
	box-shadow: var(--glow-pink);
}

#search-status {
	color: var(--neon-cyan);
	margin-bottom: 18px;
	font-size: 0.875rem;
	text-shadow: 0 0 8px rgba(0, 255, 249, 0.4);
}

#search-results {
	max-height: 60vh;
	overflow-y: auto;
}

.search-result {
	margin-bottom: 18px;
	border: 1px solid var(--border-subtle);
	border-radius: 4px;
	overflow: hidden;
}

.result-page {
	background: rgba(45, 27, 78, 0.6);
	padding: 12px 18px;
	font-family: 'Orbitron', sans-serif;
	font-weight: 600;
	font-size: 0.75rem;
	color: var(--neon-magenta);
	text-shadow: 0 0 8px rgba(242, 34, 255, 0.6);
	border-bottom: 1px solid var(--border-subtle);
	letter-spacing: 0.1em;
	text-transform: uppercase;
}

.image-block {
	margin: 18px 0;
}

.image-block img {
	max-width: 100%;
	border-radius: 4px;
	border: 1px solid var(--grid-color);
	box-shadow: 0 0 20px rgba(242, 34, 255, 0.2);
}

a {
	color: var(--neon-cyan);
	text-shadow: 0 0 5px rgba(0, 255, 249, 0.3);
	transition: all 0.3s ease;
}

a:hover {
	text-shadow: var(--glow-cyan);
}

/* Scrollbar styling - synthwave themed */
::-webkit-scrollbar {
	width: 10px;
	height: 10px;
}

::-webkit-scrollbar-track {
	background: var(--bg-secondary);
	border-left: 1px solid var(--grid-color);
}

::-webkit-scrollbar-thumb {
	background: linear-gradient(180deg, var(--neon-magenta) 0%, var(--neon-pink) 100%);
	border-radius: 0;
	box-shadow: 0 0 10px rgba(242, 34, 255, 0.5);
}

::-webkit-scrollbar-thumb:hover {
	background: linear-gradient(180deg, var(--neon-pink) 0%, var(--neon-magenta) 100%);
}

/* Selection - hot pink */
::selection {
	background: rgba(255, 41, 117, 0.4);
	color: #fff;
	text-shadow: 0 0 10px rgba(255, 41, 117, 0.8);
}
`
}
