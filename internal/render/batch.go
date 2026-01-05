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
:root {
	--bg-color: #ffffff;
	--text-color: #333333;
	--link-color: #1976d2;
	--border-color: #e0e0e0;
	--hover-bg: #f5f5f5;
	--text-muted: #757575;
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

h1 a:hover {
	text-decoration: underline;
}

.stats {
	color: var(--text-muted);
	margin-bottom: 20px;
}

.index-items {
	margin: 20px 0;
}

.index-item {
	display: block;
	padding: 15px;
	border: 1px solid var(--border-color);
	border-radius: 8px;
	margin-bottom: 10px;
	text-decoration: none;
	color: inherit;
}

.index-item:hover {
	background: var(--hover-bg);
}

.index-item-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
}

.project-name, .session-date {
	font-weight: bold;
	color: var(--link-color);
}

.project-date, .session-size {
	color: var(--text-muted);
	font-size: 0.9em;
}

.project-stats, .session-summary {
	color: var(--text-muted);
	font-size: 0.9em;
	margin-top: 5px;
}

.back-link {
	color: var(--link-color);
	text-decoration: none;
}

.back-link:hover {
	text-decoration: underline;
}
`
}

// FormatDate formats a time for display
func FormatDate(t time.Time) string {
	return t.Format("Jan 2, 2006")
}
