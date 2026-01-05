package session

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionInfo contains metadata about a discovered session
type SessionInfo struct {
	Path        string
	ProjectName string
	SessionID   string
	ModTime     time.Time
	Size        int64
	Summary     string
}

// ProjectInfo contains metadata about a project folder
type ProjectInfo struct {
	Name     string
	Path     string
	Sessions []SessionInfo
	ModTime  time.Time
}

// GetClaudeProjectsDir returns the path to Claude's projects directory
func GetClaudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// FindLocalSessions finds all local session files
func FindLocalSessions(limit int) ([]SessionInfo, error) {
	projectsDir, err := GetClaudeProjectsDir()
	if err != nil {
		return nil, err
	}

	var sessions []SessionInfo

	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Get project name from path
		rel, _ := filepath.Rel(projectsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		projectName := ""
		if len(parts) > 1 {
			projectName = parts[0]
		}

		// Get session ID from filename
		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

		sessions = append(sessions, SessionInfo{
			Path:        path,
			ProjectName: projectName,
			SessionID:   sessionID,
			ModTime:     info.ModTime(),
			Size:        info.Size(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking projects directory: %w", err)
	}

	// Sort by modification time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	// Apply limit
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

// FindAllSessions finds all sessions organized by project
func FindAllSessions() ([]ProjectInfo, error) {
	projectsDir, err := GetClaudeProjectsDir()
	if err != nil {
		return nil, err
	}

	projectMap := make(map[string]*ProjectInfo)

	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Get project name from path
		rel, _ := filepath.Rel(projectsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		projectName := ""
		if len(parts) > 1 {
			projectName = parts[0]
		}

		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

		sessionInfo := SessionInfo{
			Path:        path,
			ProjectName: projectName,
			SessionID:   sessionID,
			ModTime:     info.ModTime(),
			Size:        info.Size(),
		}

		if _, ok := projectMap[projectName]; !ok {
			projectMap[projectName] = &ProjectInfo{
				Name: projectName,
				Path: filepath.Dir(path),
			}
		}

		projectMap[projectName].Sessions = append(projectMap[projectName].Sessions, sessionInfo)
		if sessionInfo.ModTime.After(projectMap[projectName].ModTime) {
			projectMap[projectName].ModTime = sessionInfo.ModTime
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking projects directory: %w", err)
	}

	// Convert map to slice
	var projects []ProjectInfo
	for _, p := range projectMap {
		// Sort sessions within each project
		sort.Slice(p.Sessions, func(i, j int) bool {
			return p.Sessions[i].ModTime.After(p.Sessions[j].ModTime)
		})
		projects = append(projects, *p)
	}

	// Sort projects by most recent activity
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ModTime.After(projects[j].ModTime)
	})

	return projects, nil
}

// GetSessionSummary loads a session and returns the first user message as summary
func GetSessionSummary(path string) (string, error) {
	session, err := ParseFile(path)
	if err != nil {
		return "", err
	}

	summary := GetFirstUserMessage(session)
	if len(summary) > 100 {
		summary = summary[:100] + "..."
	}
	return summary, nil
}

// LoadSessionSummaries loads summaries for a list of sessions
func LoadSessionSummaries(sessions []SessionInfo) {
	for i := range sessions {
		summary, err := GetSessionSummary(sessions[i].Path)
		if err == nil {
			sessions[i].Summary = summary
		}
	}
}

// SearchMatch represents a single match within a session
type SearchMatch struct {
	Text    string // The matching text with context
	Context string // "user" or "assistant"
}

// SearchResult represents search results for a single session
type SearchResult struct {
	SessionInfo SessionInfo
	Matches     []SearchMatch
}

// SearchSessions searches all sessions for a query string
func SearchSessions(query string) ([]SearchResult, error) {
	projectsDir, err := GetClaudeProjectsDir()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []SearchResult

	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Search this session file
		matches, err := searchSessionFile(path, query)
		if err != nil || len(matches) == 0 {
			return nil
		}

		// Get project name from path
		rel, _ := filepath.Rel(projectsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		projectName := ""
		if len(parts) > 1 {
			projectName = parts[0]
		}

		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

		results = append(results, SearchResult{
			SessionInfo: SessionInfo{
				Path:        path,
				ProjectName: projectName,
				SessionID:   sessionID,
				ModTime:     info.ModTime(),
				Size:        info.Size(),
			},
			Matches: matches,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("searching sessions: %w", err)
	}

	// Sort by modification time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].SessionInfo.ModTime.After(results[j].SessionInfo.ModTime)
	})

	return results, nil
}

// searchSessionFile searches a single session file for the query
func searchSessionFile(path, query string) ([]SearchMatch, error) {
	session, err := ParseFile(path)
	if err != nil {
		return nil, err
	}

	var matches []SearchMatch

	for _, msg := range session.Messages {
		text := ExtractText(&msg)
		textLower := strings.ToLower(text)

		if strings.Contains(textLower, query) {
			// Find the match and extract context
			snippet := extractSnippet(text, query, 60)
			if snippet != "" {
				matches = append(matches, SearchMatch{
					Text:    snippet,
					Context: msg.Role,
				})
			}
		}
	}

	return matches, nil
}

// extractSnippet extracts a snippet around the query match
func extractSnippet(text, query string, contextChars int) string {
	textLower := strings.ToLower(text)
	queryLower := strings.ToLower(query)

	idx := strings.Index(textLower, queryLower)
	if idx == -1 {
		return ""
	}

	start := idx - contextChars
	if start < 0 {
		start = 0
	}

	end := idx + len(query) + contextChars
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]

	// Clean up whitespace
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	snippet = strings.ReplaceAll(snippet, "\t", " ")
	for strings.Contains(snippet, "  ") {
		snippet = strings.ReplaceAll(snippet, "  ", " ")
	}
	snippet = strings.TrimSpace(snippet)

	// Add ellipsis
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}

	return snippet
}
