package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	// Just verify it doesn't error
	if err := Run([]string{"help"}); err != nil {
		t.Errorf("help command failed: %v", err)
	}

	if err := Run([]string{"--help"}); err != nil {
		t.Errorf("--help failed: %v", err)
	}

	if err := Run([]string{"-h"}); err != nil {
		t.Errorf("-h failed: %v", err)
	}
}

func TestRun_Version(t *testing.T) {
	if err := Run([]string{"version"}); err != nil {
		t.Errorf("version command failed: %v", err)
	}

	if err := Run([]string{"--version"}); err != nil {
		t.Errorf("--version failed: %v", err)
	}

	if err := Run([]string{"-v"}); err != nil {
		t.Errorf("-v failed: %v", err)
	}
}

func TestRun_JSON(t *testing.T) {
	// Create a test session file
	tmpFile, err := os.CreateTemp("", "session-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(`{
		"messages": [
			{"role": "user", "content": "Hello", "timestamp": "2024-01-15T10:00:00Z"},
			{"role": "assistant", "content": [{"type": "text", "text": "Hi there!"}], "timestamp": "2024-01-15T10:00:05Z"}
		]
	}`)
	tmpFile.Close()

	// Create temp output directory
	tmpDir, err := os.MkdirTemp("", "output-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Run the json command
	err = Run([]string{"json", "-o", tmpDir, "--quiet", tmpFile.Name()})
	if err != nil {
		t.Fatalf("json command failed: %v", err)
	}

	// Verify output was created
	indexPath := filepath.Join(tmpDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.html was not created")
	}

	// Verify content
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("Failed to read index.html: %v", err)
	}

	if !strings.Contains(string(content), "Claude Code transcript") {
		t.Error("Output should contain transcript title")
	}
}

func TestRun_JSON_JSONL(t *testing.T) {
	// Create a JSONL test file
	tmpFile, err := os.CreateTemp("", "session-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(`{"role": "user", "content": "Hello", "timestamp": "2024-01-15T10:00:00Z"}
{"role": "assistant", "content": [{"type": "text", "text": "Hi!"}], "timestamp": "2024-01-15T10:00:05Z"}`)
	tmpFile.Close()

	tmpDir, err := os.MkdirTemp("", "output-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = Run([]string{"json", "-o", tmpDir, "--quiet", tmpFile.Name()})
	if err != nil {
		t.Fatalf("json command failed: %v", err)
	}

	indexPath := filepath.Join(tmpDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.html was not created")
	}
}

func TestRun_JSON_NoFile(t *testing.T) {
	err := Run([]string{"json"})
	if err == nil {
		t.Error("Expected error when no file provided")
	}
}

func TestRun_JSON_NonexistentFile(t *testing.T) {
	err := Run([]string{"json", "--quiet", "/nonexistent/file.json"})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestRun_JSON_IncludeJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "session-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(`{"messages": [{"role": "user", "content": "Test", "timestamp": "2024-01-15T10:00:00Z"}]}`)
	tmpFile.Close()

	tmpDir, err := os.MkdirTemp("", "output-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = Run([]string{"json", "-o", tmpDir, "--include-json", "--quiet", tmpFile.Name()})
	if err != nil {
		t.Fatalf("json command failed: %v", err)
	}

	// Check that the source file was copied
	copiedPath := filepath.Join(tmpDir, filepath.Base(tmpFile.Name()))
	if _, err := os.Stat(copiedPath); os.IsNotExist(err) {
		t.Error("Source JSON file was not copied")
	}
}

func TestExtractGistID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://gist.github.com/abc123", "abc123"},
		{"https://gist.github.com/user/abc123def", "abc123def"},
		{"abc123", "abc123"},
	}

	for _, tt := range tests {
		result := extractGistID(tt.input)
		if result != tt.expected {
			t.Errorf("extractGistID(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestRun_All_NoSessions(t *testing.T) {
	// Create a temporary home directory without any Claude sessions
	tmpHome, err := os.MkdirTemp("", "home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	// Override HOME temporarily
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	err = Run([]string{"all", "--quiet"})
	if err == nil {
		t.Error("Expected error when no sessions exist")
	}

	if !strings.Contains(err.Error(), "no sessions found") {
		t.Errorf("Expected 'no sessions found' error, got: %v", err)
	}
}

func TestRun_Local_NoSessions(t *testing.T) {
	// Create a temporary home directory without any Claude sessions
	tmpHome, err := os.MkdirTemp("", "home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	// Override HOME temporarily
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	err = runLocal([]string{"--quiet"})
	if err == nil {
		t.Error("Expected error when no sessions exist")
	}
}

func TestRun_Web_NoSessionID(t *testing.T) {
	err := Run([]string{"web"})
	if err == nil {
		t.Error("Expected error when no session ID provided")
	}
}
