package gist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GistFile represents a file in a gist
type GistFile struct {
	Content string `json:"content"`
}

// GistRequest represents a request to create a gist
type GistRequest struct {
	Description string              `json:"description"`
	Public      bool                `json:"public"`
	Files       map[string]GistFile `json:"files"`
}

// GistResponse represents a response from the GitHub API
type GistResponse struct {
	ID      string `json:"id"`
	HTMLURL string `json:"html_url"`
}

// Upload uploads all files in a directory to GitHub Gist using gh CLI
func Upload(dir string, public bool) (string, error) {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return "", errors.New("gh CLI not found. Install from https://cli.github.com/")
	}

	// Collect all files
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking directory: %w", err)
	}

	if len(files) == 0 {
		return "", errors.New("no files to upload")
	}

	// Build gh gist create command (private by default)
	args := []string{"gist", "create"}
	if public {
		args = append(args, "--public")
	}
	for _, f := range files {
		args = append(args, f)
	}

	cmd := exec.Command("gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh gist create failed: %s", stderr.String())
	}

	// Parse gist URL from output
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}

	// Find URL in output
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "gist.github.com") {
			return strings.TrimSpace(line), nil
		}
	}

	return output, nil
}

// UploadViaAPI uploads files to GitHub Gist using the API directly
// Requires GITHUB_TOKEN environment variable
func UploadViaAPI(dir string, public bool) (string, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return "", errors.New("GITHUB_TOKEN environment variable not set")
	}

	// Collect files
	files := make(map[string]GistFile)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Use relative path as filename
		relPath, _ := filepath.Rel(dir, path)
		files[relPath] = GistFile{Content: string(content)}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("reading files: %w", err)
	}

	// Create request (private by default)
	req := GistRequest{
		Description: "Claude Code Transcript",
		Public:      public,
		Files:       files,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	// Use gh API command
	cmd := exec.Command("gh", "api", "gists", "-X", "POST", "--input", "-")
	cmd.Stdin = bytes.NewReader(body)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("API request failed: %s", stderr.String())
	}

	var resp GistResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	return resp.HTMLURL, nil
}
