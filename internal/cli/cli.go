package cli

import (
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
    --no-open            Don't open viewer after uploading
    -h, --help           Show this help message
    -v, --version        Show version

EXAMPLES:
    claude-session-export                          # Interactive picker, upload to Gist
    claude-session-export json session.jsonl      # Upload specific file to Gist
    claude-session-export web SESSION_ID          # Fetch from API, upload to Gist
    claude-session-export search "error"          # Search sessions
    claude-session-export open https://gist.github.com/user/id`)
}

func runLocal(args []string) error {
	fs := flag.NewFlagSet("local", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
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

	return exportSession(selected.Path, *outputDir, *uploadGist, !*noOpen)
}

func runJSON(args []string) error {
	fs := flag.NewFlagSet("json", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	noOpen := fs.Bool("no-open", false, "Don't open viewer after uploading")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export json <file-or-url>")
	}

	path := fs.Arg(0)

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return exportURL(path, *outputDir, *uploadGist, !*noOpen)
	}

	return exportSession(path, *outputDir, *uploadGist, !*noOpen)
}

func runWeb(args []string) error {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
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

	return exportSession(tmpFile.Name(), *outputDir, *uploadGist, !*noOpen)
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
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
	return exportSession(selected.Path, *outputDir, *uploadGist, !*noOpen)
}

func runOpen(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: claude-session-export open <gist-url>")
	}

	gistURL := args[0]
	fmt.Printf("Opening viewer for: %s\n", gistURL)
	return openGistInViewer(gistURL)
}

func exportSession(path, outputDir string, uploadGist, openBrowser bool) error {
	// Validate file exists and is readable
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("cannot access file: %w", err)
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

func exportURL(url, outputDir string, uploadGist, openBrowser bool) error {
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

	return exportSession(tmpFile.Name(), outputDir, uploadGist, openBrowser)
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

	if _, err := tmpFile.Write(viewerHTML); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing viewer: %w", err)
	}
	tmpFile.Close()

	viewerPath := tmpFile.Name()
	urlWithParam := "file://" + viewerPath + "?url=" + gistURL

	return openInBrowser(urlWithParam)
}
