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
	--neon-green: #72f1b8;

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
	--glow-magenta: 0 0 30px rgba(242, 34, 255, 0.5);

	/* Grid */
	--grid-color: rgba(242, 34, 255, 0.15);
	--grid-bright: rgba(242, 34, 255, 0.4);

	--border-subtle: rgba(242, 34, 255, 0.12);
}

* {
	box-sizing: border-box;
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

.index-items {
	margin: 28px 0;
}

.index-item {
	display: block;
	padding: 22px;
	border: 1px solid var(--border-subtle);
	border-radius: 4px;
	margin-bottom: 14px;
	text-decoration: none;
	color: inherit;
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
	align-items: center;
	gap: 18px;
}

.project-name, .session-date {
	font-family: 'Orbitron', sans-serif;
	font-weight: 600;
	color: var(--neon-cyan);
	text-shadow: 0 0 10px rgba(0, 255, 249, 0.5);
	letter-spacing: 0.05em;
}

.project-date, .session-size {
	color: var(--text-muted);
	font-size: 0.8rem;
	font-family: 'IBM Plex Mono', monospace;
}

.project-stats, .session-summary {
	color: var(--text-secondary);
	font-size: 0.85rem;
	margin-top: 10px;
}

.back-link {
	font-family: 'Orbitron', sans-serif;
	color: var(--neon-cyan);
	text-decoration: none;
	font-weight: 500;
	font-size: 0.85rem;
	letter-spacing: 0.08em;
	display: inline-flex;
	align-items: center;
	gap: 10px;
	padding: 12px 0;
	transition: all 0.3s ease;
	text-shadow: 0 0 8px rgba(0, 255, 249, 0.4);
}

.back-link:hover {
	text-shadow: var(--glow-cyan);
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

// FormatDate formats a time for display
func FormatDate(t time.Time) string {
	return t.Format("Jan 2, 2006")
}
