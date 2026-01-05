package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robzolkos/claude-session-export/internal/session"
)

func TestGenerator_Generate(t *testing.T) {
	// Create a test session
	sess := &session.Session{
		Messages: []session.Message{
			{
				Role:      "user",
				Content:   session.Content{{Type: "text", Text: "Hello, Claude!"}},
				Timestamp: time.Now(),
			},
			{
				Role:      "assistant",
				Content:   session.Content{{Type: "text", Text: "Hello! How can I help?"}},
				Timestamp: time.Now().Add(time.Second),
			},
			{
				Role:      "user",
				Content:   session.Content{{Type: "text", Text: "Write some code please."}},
				Timestamp: time.Now().Add(time.Minute),
			},
			{
				Role:      "assistant",
				Content:   session.Content{{Type: "text", Text: "Sure, here's some code."}},
				Timestamp: time.Now().Add(time.Minute + time.Second),
			},
		},
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "test-generator-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate
	gen := &Generator{
		Session:   sess,
		OutputDir: tmpDir,
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check that files were created
	indexPath := filepath.Join(tmpDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.html was not created")
	}

	pagePath := filepath.Join(tmpDir, "page-001.html")
	if _, err := os.Stat(pagePath); os.IsNotExist(err) {
		t.Error("page-001.html was not created")
	}

	// Read and verify index content
	indexContent, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("Failed to read index.html: %v", err)
	}

	if !strings.Contains(string(indexContent), "Claude Code transcript") {
		t.Error("Index should contain title")
	}

	if !strings.Contains(string(indexContent), "prompts") {
		t.Error("Index should contain stats")
	}

	// Read and verify page content
	pageContent, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("Failed to read page-001.html: %v", err)
	}

	if !strings.Contains(string(pageContent), "Hello, Claude!") {
		t.Error("Page should contain user message")
	}

	if !strings.Contains(string(pageContent), "Hello! How can I help?") {
		t.Error("Page should contain assistant response")
	}
}

func TestGenerator_Generate_WithRepoURL(t *testing.T) {
	sess := &session.Session{
		Messages: []session.Message{
			{
				Role:      "user",
				Content:   session.Content{{Type: "text", Text: "Commit the changes"}},
				Timestamp: time.Now(),
			},
			{
				Role: "assistant",
				Content: session.Content{
					{Type: "tool_result", Content: "[main abc1234] Initial commit"},
				},
				Timestamp: time.Now().Add(time.Second),
			},
		},
	}

	tmpDir, err := os.MkdirTemp("", "test-generator-repo-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gen := &Generator{
		Session:   sess,
		OutputDir: tmpDir,
		RepoURL:   "https://github.com/user/repo",
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	pageContent, err := os.ReadFile(filepath.Join(tmpDir, "page-001.html"))
	if err != nil {
		t.Fatalf("Failed to read page-001.html: %v", err)
	}

	if !strings.Contains(string(pageContent), "https://github.com/user/repo/commit/abc1234") {
		t.Error("Page should contain GitHub commit link")
	}
}

func TestGenerator_Generate_Pagination(t *testing.T) {
	// Create a session with more messages than fit on one page
	var messages []session.Message
	now := time.Now()

	// Create 12 user messages (should result in 3 pages with 5 per page)
	for i := 0; i < 12; i++ {
		messages = append(messages,
			session.Message{
				Role:      "user",
				Content:   session.Content{{Type: "text", Text: "Question " + string(rune('A'+i))}},
				Timestamp: now.Add(time.Duration(i*2) * time.Minute),
			},
			session.Message{
				Role:      "assistant",
				Content:   session.Content{{Type: "text", Text: "Answer " + string(rune('A'+i))}},
				Timestamp: now.Add(time.Duration(i*2+1) * time.Minute),
			},
		)
	}

	sess := &session.Session{Messages: messages}

	tmpDir, err := os.MkdirTemp("", "test-pagination-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gen := &Generator{
		Session:   sess,
		OutputDir: tmpDir,
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check that multiple pages were created
	for i := 1; i <= 3; i++ {
		pagePath := filepath.Join(tmpDir, "page-"+padNumber(i)+".html")
		if _, err := os.Stat(pagePath); os.IsNotExist(err) {
			t.Errorf("page-%03d.html was not created", i)
		}
	}

	// Verify pagination links in page 2
	page2Content, err := os.ReadFile(filepath.Join(tmpDir, "page-002.html"))
	if err != nil {
		t.Fatalf("Failed to read page-002.html: %v", err)
	}

	if !strings.Contains(string(page2Content), "page-001.html") {
		t.Error("Page 2 should have link to page 1")
	}

	if !strings.Contains(string(page2Content), "page-003.html") {
		t.Error("Page 2 should have link to page 3")
	}
}

func padNumber(n int) string {
	return string(rune('0'+n/100)) + string(rune('0'+(n%100)/10)) + string(rune('0'+n%10))
}

func TestGenerator_Generate_IncludeJSON(t *testing.T) {
	sess := &session.Session{
		Messages: []session.Message{
			{
				Role:      "user",
				Content:   session.Content{{Type: "text", Text: "Hello"}},
				Timestamp: time.Now(),
			},
		},
	}

	// Create a temp source file
	tmpSrc, err := os.CreateTemp("", "session-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp source: %v", err)
	}
	tmpSrc.WriteString(`{"role": "user", "content": "Hello"}`)
	tmpSrc.Close()
	defer os.Remove(tmpSrc.Name())

	tmpDir, err := os.MkdirTemp("", "test-include-json-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gen := &Generator{
		Session:     sess,
		OutputDir:   tmpDir,
		IncludeJSON: true,
		SourcePath:  tmpSrc.Name(),
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check that source file was copied
	copiedPath := filepath.Join(tmpDir, filepath.Base(tmpSrc.Name()))
	if _, err := os.Stat(copiedPath); os.IsNotExist(err) {
		t.Error("Source file was not copied")
	}
}

func TestGenerator_Generate_EmptySession(t *testing.T) {
	sess := &session.Session{Messages: []session.Message{}}

	tmpDir, err := os.MkdirTemp("", "test-empty-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gen := &Generator{
		Session:   sess,
		OutputDir: tmpDir,
	}

	err = gen.Generate()
	if err == nil {
		t.Error("Expected error for empty session")
	}
}

func TestGenerator_SearchJS(t *testing.T) {
	sess := &session.Session{
		Messages: []session.Message{
			{
				Role:      "user",
				Content:   session.Content{{Type: "text", Text: "Test"}},
				Timestamp: time.Now(),
			},
			{
				Role:      "assistant",
				Content:   session.Content{{Type: "text", Text: "Response"}},
				Timestamp: time.Now().Add(time.Second),
			},
		},
	}

	tmpDir, err := os.MkdirTemp("", "test-search-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gen := &Generator{
		Session:   sess,
		OutputDir: tmpDir,
	}

	if err := gen.Generate(); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	indexContent, err := os.ReadFile(filepath.Join(tmpDir, "index.html"))
	if err != nil {
		t.Fatalf("Failed to read index.html: %v", err)
	}

	// Check search functionality is included
	if !strings.Contains(string(indexContent), "search-input") {
		t.Error("Index should contain search input")
	}

	if !strings.Contains(string(indexContent), "search-modal") {
		t.Error("Index should contain search modal")
	}

	if !strings.Contains(string(indexContent), "doSearch") {
		t.Error("Index should contain search JavaScript")
	}
}

func TestGetCSS(t *testing.T) {
	css := getCSS()

	// Check for key CSS components
	checks := []string{
		"--user-bg",
		"--assistant-bg",
		"--thinking-bg",
		"--tool-bg",
		".message",
		".pagination",
		".tool-block",
		".commit-card",
		"@media",
	}

	for _, check := range checks {
		if !strings.Contains(css, check) {
			t.Errorf("CSS should contain %q", check)
		}
	}
}
