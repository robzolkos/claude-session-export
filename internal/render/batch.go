package render

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robzolkos/claude-session-export/internal/session"
)

// BatchGenerator generates HTML for all sessions
type BatchGenerator struct {
	OutputDir string
	Projects  []session.ProjectInfo
}

// Generate generates all project pages and master index
func (g *BatchGenerator) Generate() error {
	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Generate each project
	for _, project := range g.Projects {
		if err := g.generateProject(project); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate project %s: %v\n", project.Name, err)
			continue
		}
	}

	// Generate master index
	return g.generateMasterIndex()
}

func (g *BatchGenerator) generateProject(project session.ProjectInfo) error {
	projectDir := filepath.Join(g.OutputDir, sanitizeDirName(project.Name))
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return err
	}

	// Generate each session
	for _, sess := range project.Sessions {
		sessionDir := filepath.Join(projectDir, sess.SessionID)

		// Parse session
		s, err := session.ParseFile(sess.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse session %s: %v\n", sess.Path, err)
			continue
		}

		// Generate HTML
		gen := &Generator{
			Session:     s,
			OutputDir:   sessionDir,
			IncludeJSON: true,
			SourcePath:  sess.Path,
		}

		if err := gen.Generate(); err != nil {
			// Skip sessions with no conversations silently (e.g., agent sessions with only assistant messages)
			if err == ErrNoConversations {
				continue
			}
			fmt.Fprintf(os.Stderr, "Warning: failed to generate session %s: %v\n", sess.SessionID, err)
			continue
		}
	}

	// Generate project index
	return g.generateProjectIndex(project, projectDir)
}

func (g *BatchGenerator) generateProjectIndex(project session.ProjectInfo, projectDir string) error {
	var buf bytes.Buffer

	// Load summaries
	session.LoadSessionSummaries(project.Sessions)

	// Build sessions list
	var sessionsHTML bytes.Buffer
	for _, sess := range project.Sessions {
		summary := sess.Summary
		if summary == "" {
			summary = "(No summary available)"
		}
		if len(summary) > 100 {
			summary = summary[:100] + "..."
		}

		sessionsHTML.WriteString(fmt.Sprintf(`
			<a href="%s/index.html" class="index-item">
				<div class="index-item-header">
					<span class="session-date">%s</span>
					<span class="session-size">%.1f KB</span>
				</div>
				<div class="session-summary">%s</div>
			</a>
		`, sess.SessionID, sess.ModTime.Format("Jan 2, 2006"), float64(sess.Size)/1024, html.EscapeString(summary)))
	}

	sessionWord := "sessions"
	if len(project.Sessions) == 1 {
		sessionWord = "session"
	}

	content := fmt.Sprintf(`
		<h1><a href="../index.html">Claude Code Archive</a> / %s</h1>
		<p class="stats">%d %s</p>
		<div class="index-items">%s</div>
		<p><a href="../index.html" class="back-link">← Back to Archive</a></p>
	`, html.EscapeString(project.Name), len(project.Sessions), sessionWord, sessionsHTML.String())

	buf.WriteString(wrapArchiveHTML(project.Name+" - Claude Code Archive", content))

	return os.WriteFile(filepath.Join(projectDir, "index.html"), buf.Bytes(), 0644)
}

func (g *BatchGenerator) generateMasterIndex() error {
	var buf bytes.Buffer

	// Count total sessions
	totalSessions := 0
	for _, p := range g.Projects {
		totalSessions += len(p.Sessions)
	}

	// Build projects list
	var projectsHTML bytes.Buffer
	for _, project := range g.Projects {
		sessionWord := "sessions"
		if len(project.Sessions) == 1 {
			sessionWord = "session"
		}

		projectsHTML.WriteString(fmt.Sprintf(`
			<a href="%s/index.html" class="index-item">
				<div class="index-item-header">
					<span class="project-name">%s</span>
					<span class="project-date">%s</span>
				</div>
				<div class="project-stats">%d %s</div>
			</a>
		`, sanitizeDirName(project.Name), html.EscapeString(project.Name),
			project.ModTime.Format("Jan 2, 2006"),
			len(project.Sessions), sessionWord))
	}

	content := fmt.Sprintf(`
		<h1>Claude Code Archive</h1>
		<p class="stats">%d projects · %d sessions</p>
		<div class="index-items">%s</div>
	`, len(g.Projects), totalSessions, projectsHTML.String())

	buf.WriteString(wrapArchiveHTML("Claude Code Archive", content))

	return os.WriteFile(filepath.Join(g.OutputDir, "index.html"), buf.Bytes(), 0644)
}

func sanitizeDirName(name string) string {
	// Replace problematic characters
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	return replacer.Replace(name)
}

func wrapArchiveHTML(title, content string) string {
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
</body>
</html>`, html.EscapeString(title), getArchiveCSS(), content)
}

func getArchiveCSS() string {
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

	--text-primary: #e8e8ef;
	--text-secondary: #9898a8;
	--text-muted: #5c5c6e;

	--glow-cyan: 0 0 20px rgba(0, 240, 255, 0.4);
	--glow-pink: 0 0 20px rgba(255, 0, 128, 0.4);

	--border-subtle: rgba(255, 255, 255, 0.06);
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

.index-items {
	margin: 24px 0;
}

.index-item {
	display: block;
	padding: 20px;
	border: 1px solid var(--border-subtle);
	border-radius: 12px;
	margin-bottom: 12px;
	text-decoration: none;
	color: inherit;
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
	align-items: center;
	gap: 16px;
}

.project-name, .session-date {
	font-weight: 600;
	color: var(--neon-cyan);
}

.project-date, .session-size {
	color: var(--text-muted);
	font-size: 0.8rem;
	font-family: 'JetBrains Mono', monospace;
}

.project-stats, .session-summary {
	color: var(--text-secondary);
	font-size: 0.85rem;
	margin-top: 8px;
}

.back-link {
	color: var(--neon-cyan);
	text-decoration: none;
	font-weight: 500;
	display: inline-flex;
	align-items: center;
	gap: 8px;
	padding: 10px 0;
	transition: all 0.2s ease;
}

.back-link:hover {
	text-shadow: var(--glow-cyan);
}

a {
	color: var(--neon-cyan);
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

::selection {
	background: rgba(0, 240, 255, 0.3);
	color: var(--text-primary);
}
`
}

// FormatDate formats a time for display
func FormatDate(t time.Time) string {
	return t.Format("Jan 2, 2006")
}
