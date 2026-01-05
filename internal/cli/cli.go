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
	"github.com/robzolkos/claude-session-export/internal/render"
	"github.com/robzolkos/claude-session-export/internal/session"
	"github.com/robzolkos/claude-session-export/internal/web"
)

var version = "dev"

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
	case "all":
		return runAll(args[1:])
	case "search":
		return runSearch(args[1:])
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
	fmt.Println(`claude-session-export - Transform Claude Code sessions into shareable HTML

USAGE:
    claude-session-export [COMMAND] [OPTIONS]

COMMANDS:
    local    Browse and export local Claude Code sessions (default)
    json     Export a specific JSON or JSONL file
    web      Fetch and export sessions from Claude API
    all      Export all local sessions to a browsable archive
    search   Search across all sessions for a term

OPTIONS:
    -o, --output DIR     Output directory (default: temp directory)
    --gist               Upload to GitHub Gist (private by default)
    --public             Make gist public (use with --gist)
    --open               Open in browser when done
    --include-json       Include original JSON/JSONL file
    --quiet              Suppress output
    -h, --help           Show this help message
    -v, --version        Show version

EXAMPLES:
    claude-session-export                     # Interactive session picker
    claude-session-export json session.jsonl  # Convert specific file
    claude-session-export all --open          # Convert all sessions
    claude-session-export web SESSION_ID      # Fetch from API
    claude-session-export search "burrito"    # Search all sessions`)
}

func runLocal(args []string) error {
	fs := flag.NewFlagSet("local", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	publicGist := fs.Bool("public", false, "Make gist public")
	openBrowser := fs.Bool("open", false, "Open in browser when done")
	includeJSON := fs.Bool("include-json", false, "Include original JSON file")
	quiet := fs.Bool("quiet", false, "Suppress output")
	limit := fs.Int("limit", 20, "Maximum number of sessions to show")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Find local sessions
	sessions, err := session.FindLocalSessions(*limit)
	if err != nil {
		return fmt.Errorf("finding sessions: %w", err)
	}

	if len(sessions) == 0 {
		return errors.New("no sessions found in ~/.claude/projects")
	}

	// Load summaries for display
	session.LoadSessionSummaries(sessions)

	// Interactive selection
	selected, err := selectSession(sessions)
	if err != nil {
		return err
	}

	return convertSession(selected.Path, *outputDir, *uploadGist, *publicGist, *openBrowser, *includeJSON, *quiet)
}

func runJSON(args []string) error {
	fs := flag.NewFlagSet("json", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	publicGist := fs.Bool("public", false, "Make gist public")
	openBrowser := fs.Bool("open", false, "Open in browser when done")
	includeJSON := fs.Bool("include-json", false, "Include original JSON file")
	quiet := fs.Bool("quiet", false, "Suppress output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export json <file-or-url>")
	}

	path := fs.Arg(0)

	// Check if it's a URL
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return convertURL(path, *outputDir, *uploadGist, *publicGist, *openBrowser, *quiet)
	}

	return convertSession(path, *outputDir, *uploadGist, *publicGist, *openBrowser, *includeJSON, *quiet)
}

func runWeb(args []string) error {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	publicGist := fs.Bool("public", false, "Make gist public")
	openBrowser := fs.Bool("open", false, "Open in browser when done")
	includeJSON := fs.Bool("include-json", false, "Include original JSON file")
	quiet := fs.Bool("quiet", false, "Suppress output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export web <session-id>")
	}

	sessionID := fs.Arg(0)

	// Fetch session from API
	if !*quiet {
		fmt.Printf("Fetching session %s from API...\n", sessionID)
	}

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

	return convertSession(tmpFile.Name(), *outputDir, *uploadGist, *publicGist, *openBrowser, *includeJSON, *quiet)
}

func runAll(args []string) error {
	fs := flag.NewFlagSet("all", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	openBrowser := fs.Bool("open", false, "Open in browser when done")
	quiet := fs.Bool("quiet", false, "Suppress output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Find all sessions organized by project
	projects, err := session.FindAllSessions()
	if err != nil {
		return fmt.Errorf("finding sessions: %w", err)
	}

	if len(projects) == 0 {
		return errors.New("no sessions found in ~/.claude/projects")
	}

	// Determine output directory
	outDir := *outputDir
	if outDir == "" {
		var err error
		outDir, err = os.MkdirTemp("", "claude-archive-*")
		if err != nil {
			return fmt.Errorf("creating temp directory: %w", err)
		}
	}

	if !*quiet {
		totalSessions := 0
		for _, p := range projects {
			totalSessions += len(p.Sessions)
		}
		fmt.Printf("Converting %d projects with %d sessions...\n", len(projects), totalSessions)
	}

	// Generate batch
	gen := &render.BatchGenerator{
		OutputDir: outDir,
		Projects:  projects,
	}

	if err := gen.Generate(); err != nil {
		return fmt.Errorf("generating archive: %w", err)
	}

	indexPath := filepath.Join(outDir, "index.html")
	if !*quiet {
		fmt.Printf("Archive created: %s\n", indexPath)
	}

	if *openBrowser {
		openInBrowser(indexPath)
	}

	return nil
}

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	outputDir := fs.String("o", "", "Output directory")
	fs.StringVar(outputDir, "output", "", "Output directory")
	uploadGist := fs.Bool("gist", false, "Upload to GitHub Gist")
	publicGist := fs.Bool("public", false, "Make gist public")
	openBrowser := fs.Bool("open", false, "Open in browser when done")
	includeJSON := fs.Bool("include-json", false, "Include original JSON file")
	quiet := fs.Bool("quiet", false, "Suppress output")
	maxMatches := fs.Int("max-matches", 3, "Maximum matches to show per session")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return errors.New("usage: claude-session-export search <query>")
	}

	query := fs.Arg(0)

	if !*quiet {
		fmt.Printf("Searching for \"%s\"...\n", query)
	}

	results, err := session.SearchSessions(query)
	if err != nil {
		return fmt.Errorf("searching sessions: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No sessions found containing \"%s\"\n", query)
		return nil
	}

	// Display results with snippets
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

		// Show up to maxMatches snippets
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

	// Interactive selection
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
	return convertSession(selected.Path, *outputDir, *uploadGist, *publicGist, *openBrowser, *includeJSON, *quiet)
}

func convertSession(path, outputDir string, uploadGist, publicGist, openBrowser, includeJSON, quiet bool) error {
	// Parse session
	sess, err := session.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parsing session: %w", err)
	}

	// Determine output directory
	outDir := outputDir
	if outDir == "" {
		var err error
		outDir, err = os.MkdirTemp("", "claude-transcript-*")
		if err != nil {
			return fmt.Errorf("creating temp directory: %w", err)
		}
	}

	// Generate HTML
	gen := &render.Generator{
		Session:     sess,
		OutputDir:   outDir,
		IncludeJSON: includeJSON,
		SourcePath:  path,
	}

	if err := gen.Generate(); err != nil {
		return fmt.Errorf("generating HTML: %w", err)
	}

	indexPath := filepath.Join(outDir, "index.html")

	if uploadGist {
		// Upload to GitHub Gist
		visibility := "private"
		if publicGist {
			visibility = "public"
		}
		if !quiet {
			fmt.Printf("Uploading to GitHub Gist (%s)...\n", visibility)
		}

		gistURL, err := gist.Upload(outDir, publicGist)
		if err != nil {
			return fmt.Errorf("uploading gist: %w", err)
		}

		if !quiet {
			fmt.Printf("Gist created: %s\n", gistURL)
			fmt.Printf("Preview: https://gistpreview.github.io/?%s/index.html\n", extractGistID(gistURL))
		}

		if openBrowser {
			previewURL := fmt.Sprintf("https://gistpreview.github.io/?%s/index.html", extractGistID(gistURL))
			openInBrowser(previewURL)
		}
	} else {
		if !quiet {
			fmt.Printf("Transcript created: %s\n", indexPath)
		}

		if openBrowser {
			openInBrowser(indexPath)
		}
	}

	return nil
}

func convertURL(url, outputDir string, uploadGist, publicGist, openBrowser, quiet bool) error {
	if !quiet {
		fmt.Printf("Fetching %s...\n", url)
	}

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

	// Create temp file
	tmpFile, err := os.CreateTemp("", "session-*.json")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmpFile.Close()

	return convertSession(tmpFile.Name(), outputDir, uploadGist, publicGist, openBrowser, false, quiet)
}

func selectSession(sessions []session.SessionInfo) (*session.SessionInfo, error) {
	if len(sessions) == 0 {
		return nil, errors.New("no sessions to select")
	}

	fmt.Println("\nSelect a session:")
	fmt.Println()

	for i, s := range sessions {
		summary := s.Summary
		if len(summary) > 60 {
			summary = summary[:60] + "..."
		}
		if summary == "" {
			summary = "(No summary)"
		}
		fmt.Printf("  %2d. [%s] %s\n", i+1, s.ModTime.Format("Jan 02"), summary)
		fmt.Printf("      %s\n", s.ProjectName)
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

func extractGistID(gistURL string) string {
	parts := strings.Split(gistURL, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}
