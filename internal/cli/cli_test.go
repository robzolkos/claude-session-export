package cli

import (
	"os"
	"path/filepath"
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

func TestRun_JSON_NoFile(t *testing.T) {
	err := Run([]string{"json"})
	if err == nil {
		t.Error("Expected error when no file provided")
	}
}

func TestRun_JSON_NonexistentFile(t *testing.T) {
	err := Run([]string{"json", "/nonexistent/file.json"})
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestRun_JSON_OutputDir(t *testing.T) {
	// Create a test JSONL file
	tmpFile, err := os.CreateTemp("", "session-*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Hello"}]},"timestamp":"2024-01-15T10:00:00Z"}`)
	tmpFile.Close()

	// Create temp output directory
	tmpDir, err := os.MkdirTemp("", "output-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Run the json command with output dir (no gist upload)
	err = Run([]string{"json", "-o", tmpDir, tmpFile.Name()})
	if err != nil {
		t.Fatalf("json command failed: %v", err)
	}

	// Verify JSONL was copied to output dir
	outputPath := filepath.Join(tmpDir, filepath.Base(tmpFile.Name()))
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("JSONL file was not copied to output directory")
	}
}

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "flags after file",
			input:    []string{"file.jsonl", "--gist", "--open"},
			expected: []string{"--gist", "--open", "file.jsonl"},
		},
		{
			name:     "flags before file",
			input:    []string{"--gist", "--open", "file.jsonl"},
			expected: []string{"--gist", "--open", "file.jsonl"},
		},
		{
			name:     "flag with value",
			input:    []string{"-o", "outdir", "file.jsonl"},
			expected: []string{"-o", "outdir", "file.jsonl"},
		},
		{
			name:     "flag with value after file",
			input:    []string{"file.jsonl", "-o", "outdir"},
			expected: []string{"-o", "outdir", "file.jsonl"},
		},
		{
			name:     "mixed flags and file",
			input:    []string{"--quiet", "file.jsonl", "--gist", "-o", "dir"},
			expected: []string{"--quiet", "--gist", "-o", "dir", "file.jsonl"},
		},
		{
			name:     "flag with equals",
			input:    []string{"file.jsonl", "--output=dir"},
			expected: []string{"--output=dir", "file.jsonl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reorderArgs(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("reorderArgs(%v) = %v, expected %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("reorderArgs(%v) = %v, expected %v", tt.input, result, tt.expected)
					return
				}
			}
		})
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

	err = runLocal([]string{})
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
