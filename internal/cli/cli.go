package cli

import (
	"archive/zip"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/robzolkos/claude-session-export/internal/gist"
	"github.com/robzolkos/claude-session-export/internal/session"
	"github.com/robzolkos/claude-session-export/internal/web"
)

var version = "dev"

// reorderArgs moves flags before positional arguments so Go's flag package can parse them.
func reorderArgs(args []string) []string {
	valueFlags := map[string]bool{
		"-o": true, "--output": true,
		"--limit": true, "--max-matches": true,
	}

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && valueFlags[arg] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, arg)
		}
	}
	return append(flags, positional...)
}

// Run executes the CLI with the given arguments
func Run(args []string) error {
	if len(args) == 0 {
		return runLocal([]string{})
	}

	switch args[0] {
	case "local":
		return runLocal(args[1:])
	case "json":
		return runJSON(args[1:])
	case "web":
		return runWeb(args[1:])
	case "search":
		return runSearch(args[1:])
	case "open":
		return runOpen(args[1:])
	case "version", "--version", "-v":
		fmt.Printf("claude-session-export %s\n", version)
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		// Treat as local command
		return runLocal(args)
	}
}

func printHelp() {
	fmt.Println(`claude-session-export - Export Claude Code sessions to GitHub Gist

USAGE:
    claude-session-export [COMMAND] [OPTIONS]

COMMANDS:
    local    Browse and export local Claude Code sessions (default)
    json     Export a specific JSONL file
    web      Fetch and export sessions from Claude API
    search   Search across all sessions for a term
    open     Open a gist URL in the session viewer

OPTIONS:
    -o, --output DIR     Save JSONL locally instead of uploading to Gist
    --zip                Create a zip file with viewer and session data
    --no-open            Don't open viewer after uploading
    -h, --help           Show this help message
    -v, --version        Show version

EXAMPLES:
    claude-session-export                          # Interactive picker, upload to Gist
    claude-session-export json session.jsonl      # Upload specific file to Gist
    claude-session-export --zip                   # Create shareable zip file
    claude-session-export web SESSION_ID          # Fetch from API, upload to Gist
    claude-session-export search "error"          # Search sessions
    claude-session-export open https://gist.github.com/user/id`)
}

func runLocal(args []string) error {
	fs := flag.NewFlagSet("local", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	createZip := fs.Bool("zip", false, "Create a zip file with viewer and session")
	noOpen := fs.Bool("no-open", false, "Don't open viewer after uploading")
	limit := fs.Int("limit", 30, "Maximum number of sessions to show")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	sessions, err := session.FindLocalSessions(*limit)
	if err != nil {
		return fmt.Errorf("finding sessions: %w", err)
	}

	if len(sessions) == 0 {
		return errors.New("no sessions found in ~/.claude/projects")
	}

	session.LoadSessionSummaries(sessions)

	selected, err := selectSession(sessions)
	if err != nil {
		return err
	}

	return exportSession(selected.Path, *outputDir, *uploadGist, *createZip, !*noOpen)
}

func runJSON(args []string) error {
	fs := flag.NewFlagSet("json", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	createZip := fs.Bool("zip", false, "Create a zip file with viewer and session")
	noOpen := fs.Bool("no-open", false, "Don't open viewer after uploading")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export json <file-or-url>")
	}

	path := fs.Arg(0)

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return exportURL(path, *outputDir, *uploadGist, *createZip, !*noOpen)
	}

	return exportSession(path, *outputDir, *uploadGist, *createZip, !*noOpen)
}

func runWeb(args []string) error {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	createZip := fs.Bool("zip", false, "Create a zip file with viewer and session")
	noOpen := fs.Bool("no-open", false, "Don't open viewer after uploading")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export web <session-id>")
	}

	sessionID := fs.Arg(0)

	fmt.Printf("Fetching session %s from API...\n", sessionID)

	sess, err := web.FetchSession(sessionID)
	if err != nil {
		return fmt.Errorf("fetching session: %w", err)
	}

	// Create temp file with session data
	tmpFile, err := os.CreateTemp("", "session-*.jsonl")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(sess); err != nil {
		return fmt.Errorf("writing session data: %w", err)
	}
	tmpFile.Close()

	return exportSession(tmpFile.Name(), *outputDir, *uploadGist, *createZip, !*noOpen)
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	createZip := fs.Bool("zip", false, "Create a zip file with viewer and session")
	noOpen := fs.Bool("no-open", false, "Don't open viewer after uploading")
	maxMatches := fs.Int("max-matches", 3, "Maximum matches to show per session")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export search <query>")
	}

	query := fs.Arg(0)

	fmt.Printf("Searching for \"%s\"...\n", query)

	results, err := session.SearchSessions(query)
	if err != nil {
		return fmt.Errorf("searching sessions: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No sessions found containing \"%s\"\n", query)
		return nil
	}

	fmt.Printf("\nFound \"%s\" in %d sessions:\n\n", query, len(results))

	for i, result := range results {
		matchCount := len(result.Matches)
		matchWord := "match"
		if matchCount > 1 {
			matchWord = "matches"
		}
		fmt.Printf("%2d. [%s] %s (%d %s)\n",
			i+1,
			result.SessionInfo.ModTime.Format("Jan 02"),
			result.SessionInfo.ProjectName,
			matchCount,
			matchWord)

		showCount := matchCount
		if showCount > *maxMatches {
			showCount = *maxMatches
		}
		for j := 0; j < showCount; j++ {
			fmt.Printf("    \"%s\"\n", result.Matches[j].Text)
		}
		if matchCount > *maxMatches {
			fmt.Printf("    ... and %d more matches\n", matchCount-*maxMatches)
		}
		fmt.Println()
	}

	fmt.Print("Enter number to export (or q to quit): ")

	var input string
	fmt.Scanln(&input)

	if input == "q" || input == "Q" {
		return nil
	}

	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err != nil || idx < 1 || idx > len(results) {
		return errors.New("invalid selection")
	}

	selected := results[idx-1].SessionInfo
	return exportSession(selected.Path, *outputDir, *uploadGist, *createZip, !*noOpen)
}

func runOpen(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: claude-session-export open <gist-url>")
	}

	gistURL := args[0]
	fmt.Printf("Opening viewer for: %s\n", gistURL)
	return openGistInViewer(gistURL)
}

func exportSession(path, outputDir string, uploadGist, createZip, openBrowser bool) error {
	// Validate file exists and is readable
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}

	// Handle zip export
	if createZip {
		return exportAsZip(path, outputDir)
	}

	// Default to gist upload unless output dir is specified
	if !uploadGist && outputDir == "" {
		uploadGist = true
		openBrowser = true
	}

	if uploadGist {
		// Create temp dir with just the JSONL file
		tmpDir, err := os.MkdirTemp("", "claude-gist-*")
		if err != nil {
			return fmt.Errorf("creating temp directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		// Copy JSONL file to temp dir
		srcData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading source file: %w", err)
		}

		destPath := filepath.Join(tmpDir, "session.jsonl")
		if err := os.WriteFile(destPath, srcData, 0644); err != nil {
			return fmt.Errorf("writing temp file: %w", err)
		}

		fmt.Println("Uploading to GitHub Gist...")

		gistURL, err := gist.Upload(tmpDir, false)
		if err != nil {
			return fmt.Errorf("uploading gist: %w", err)
		}

		fmt.Printf("Gist created: %s\n", gistURL)

		if openBrowser {
			if err := openGistInViewer(gistURL); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not open viewer: %v\n", err)
			}
		}
	} else {
		// Just copy to output dir if specified
		if outputDir != "" {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			srcData, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading source file: %w", err)
			}

			destPath := filepath.Join(outputDir, filepath.Base(path))
			if err := os.WriteFile(destPath, srcData, 0644); err != nil {
				return fmt.Errorf("writing output file: %w", err)
			}

			fmt.Printf("Session exported: %s\n", destPath)
		} else {
			fmt.Printf("Session: %s\n", path)
			fmt.Println("Use --gist to upload to GitHub Gist, or -o to specify output directory")
		}
	}

	return nil
}

func exportAsZip(sessionPath, outputDir string) error {
	// Read session data
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		return fmt.Errorf("reading session file: %w", err)
	}

	// Parse session to get project name and timestamp for filename
	sess, _ := session.ParseFile(sessionPath)
	details, _ := session.GetSessionDetails(sessionPath)

	// Build filename: project-date-time.zip
	projectName := "session"
	if sess != nil && len(sess.Messages) > 0 && sess.Messages[0].Cwd != "" {
		// Extract project name from cwd
		projectName = filepath.Base(sess.Messages[0].Cwd)
	}
	// Clean project name for filename
	projectName = strings.ReplaceAll(projectName, " ", "-")
	projectName = strings.ReplaceAll(projectName, "/", "-")

	timestamp := time.Now()
	if details != nil && !details.EndTime.IsZero() {
		timestamp = details.EndTime
	}
	dateStr := timestamp.Local().Format("2006-01-02-1504")

	zipFilename := fmt.Sprintf("%s-%s.zip", projectName, dateStr)

	// Generate local viewer HTML with embedded session data
	localViewer := generateLocalViewerHTML(sessionData)

	// Determine output path
	zipPath := zipFilename
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
		zipPath = filepath.Join(outputDir, zipFilename)
	}

	// Create zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add viewer.html to zip (session data is embedded in the HTML)
	viewerWriter, err := zipWriter.Create("viewer.html")
	if err != nil {
		return fmt.Errorf("adding viewer to zip: %w", err)
	}
	if _, err := viewerWriter.Write([]byte(localViewer)); err != nil {
		return fmt.Errorf("writing viewer to zip: %w", err)
	}

	fmt.Printf("Created: %s\n", zipPath)
	fmt.Println("Extract the zip and open viewer.html in a browser.")

	return nil
}

func generateLocalViewerHTML(sessionData []byte) string {
	// Start with the embedded viewer HTML
	html := string(viewerHTML)

	// Remove the URL input form
	html = strings.Replace(html,
		`<div class="url-form">`,
		`<div class="url-form" style="display:none;">`, 1)

	// Escape the session data for embedding in JavaScript
	// We'll base64 encode it to avoid any escaping issues
	encodedData := base64.StdEncoding.EncodeToString(sessionData)

	// Inject JavaScript to load embedded data directly (no fetch needed)
	localLoadScript := fmt.Sprintf(`
	<script>
		window.LOCAL_MODE = true;
		window.EMBEDDED_SESSION = atob("%s");
		window.addEventListener('DOMContentLoaded', function() {
			try {
				// Parse and render the embedded session data
				parseJsonl(window.EMBEDDED_SESSION);
				calculateActiveTime();
				renderStats();
				renderMessages();
				document.getElementById('session-stats').classList.add('visible');
			} catch (err) {
				document.getElementById('status').textContent = 'Error: ' + err.message;
				document.getElementById('status').className = 'status error';
			}
		});
	</script>`, encodedData)

	// Insert before </head>
	html = strings.Replace(html, "</head>", localLoadScript+"</head>", 1)

	// Remove the URL param auto-load at the end since we handle it ourselves
	html = strings.Replace(html,
		`// URL param support (from query string or injected by CLI)
		const params = new URLSearchParams(window.location.search);
		const urlParam = params.get('url') || window.GIST_URL;
		if (urlParam) {
			document.getElementById('gist-url').value = urlParam;
			loadSession();
		}`,
		`// Local mode - loading handled by LOCAL_MODE script`, 1)

	return html
}

func exportURL(url, outputDir string, uploadGist, createZip, openBrowser bool) error {
	fmt.Printf("Fetching %s...\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "session-*.jsonl")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmpFile.Close()

	return exportSession(tmpFile.Name(), outputDir, uploadGist, createZip, openBrowser)
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
)

func selectSession(sessions []session.SessionInfo) (*session.SessionInfo, error) {
	if len(sessions) == 0 {
		return nil, errors.New("no sessions to select")
	}

	// Calculate max project name width for alignment
	maxProjectWidth := 0
	projectNames := make([]string, len(sessions))
	for i, s := range sessions {
		projectNames[i] = formatProjectName(s.ProjectName)
		if len(projectNames[i]) > maxProjectWidth {
			maxProjectWidth = len(projectNames[i])
		}
	}
	// Cap at reasonable width
	if maxProjectWidth > 30 {
		maxProjectWidth = 30
	}

	fmt.Println("\nSelect a session:")
	fmt.Println()

	for i, s := range sessions {
		projectName := projectNames[i]
		if len(projectName) > 30 {
			projectName = projectName[:27] + "..."
		}

		// Format time (use EndTime - last message time) in local timezone
		displayTime := s.ModTime.Local()
		if !s.EndTime.IsZero() {
			displayTime = s.EndTime.Local()
		}
		timeStr := displayTime.Format("Jan 02 3:04pm")

		// Build prompt count string
		promptStr := ""
		if s.UserMsgCount > 0 {
			promptStr = fmt.Sprintf("%4d prompts", s.UserMsgCount)
		}

		// Summary - truncate to fit
		summary := s.Summary
		if summary == "" {
			summary = "(No summary available)"
		}
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}

		// Columnar: num | date | project (padded) | prompts | summary
		fmt.Printf("  %2d. %s%14s%s %s%-*s%s %s%11s%s  %s%s%s\n",
			i+1,
			colorDim, timeStr, colorReset,
			colorCyan+colorBold, maxProjectWidth, projectName, colorReset,
			colorDim, promptStr, colorReset,
			colorDim, summary, colorReset)
	}

	fmt.Println()
	fmt.Print("Enter number (or q to quit): ")

	var input string
	fmt.Scanln(&input)

	if input == "q" || input == "Q" {
		return nil, errors.New("cancelled")
	}

	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err != nil || idx < 1 || idx > len(sessions) {
		return nil, errors.New("invalid selection")
	}

	return &sessions[idx-1], nil
}

// formatProjectName cleans up project path for display
func formatProjectName(name string) string {
	// Remove common prefixes like -home-username-code-
	name = strings.TrimPrefix(name, "-home-")

	// Find the last meaningful part after -code- or similar
	if idx := strings.LastIndex(name, "-code-"); idx != -1 {
		name = name[idx+6:] // Skip "-code-"
	} else if idx := strings.LastIndex(name, "-"); idx != -1 {
		// Try to get just the last component if it looks like a project name
		parts := strings.Split(name, "-")
		if len(parts) > 2 {
			// Take last 1-2 parts
			name = strings.Join(parts[len(parts)-2:], "/")
		}
	}

	// Replace remaining dashes with slashes for readability
	name = strings.ReplaceAll(name, "-", "/")

	if name == "" {
		return "~"
	}

	return name
}

func openInBrowser(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}

func openGistInViewer(gistURL string) error {
	tmpFile, err := os.CreateTemp("", "session-viewer-*.html")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	// Inject the gist URL into the HTML so it auto-loads
	// Replace a placeholder or inject a script that sets the URL
	html := string(viewerHTML)
	injection := fmt.Sprintf(`<script>window.GIST_URL = %q;</script>`, gistURL)
	html = strings.Replace(html, "</head>", injection+"</head>", 1)

	if _, err := tmpFile.WriteString(html); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing viewer: %w", err)
	}
	tmpFile.Close()

	return openInBrowser(tmpFile.Name())
}
