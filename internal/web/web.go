package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	apiBaseURL = "https://api.claude.ai"
)

// Config represents Claude configuration
type Config struct {
	OrgUUID string `json:"org_uuid"`
}

// SessionsResponse represents the API response for sessions
type SessionsResponse struct {
	Sessions []SessionMeta `json:"sessions"`
}

// SessionMeta represents session metadata from the API
type SessionMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FetchSession fetches a session from the Claude API
func FetchSession(sessionID string) ([]byte, error) {
	token, err := getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	orgUUID, err := getOrgUUID()
	if err != nil {
		return nil, fmt.Errorf("getting org UUID: %w", err)
	}

	url := fmt.Sprintf("%s/api/organizations/%s/chat_conversations/%s/full", apiBaseURL, orgUUID, sessionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// FetchSessions fetches all sessions from the Claude API
func FetchSessions() ([]SessionMeta, error) {
	token, err := getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	orgUUID, err := getOrgUUID()
	if err != nil {
		return nil, fmt.Errorf("getting org UUID: %w", err)
	}

	url := fmt.Sprintf("%s/api/organizations/%s/chat_conversations", apiBaseURL, orgUUID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var sessionsResp SessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionsResp); err != nil {
		return nil, err
	}

	return sessionsResp.Sessions, nil
}

// getAccessToken retrieves the access token from keychain (macOS) or config
func getAccessToken() (string, error) {
	// Try environment variable first
	if token := os.Getenv("CLAUDE_ACCESS_TOKEN"); token != "" {
		return token, nil
	}

	// Try macOS keychain
	if runtime.GOOS == "darwin" {
		token, err := getFromKeychain("api.claude.ai", "Claude")
		if err == nil && token != "" {
			return token, nil
		}
	}

	// Try config file
	configPath := filepath.Join(os.Getenv("HOME"), ".claude.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var config map[string]interface{}
		if json.Unmarshal(data, &config) == nil {
			if token, ok := config["access_token"].(string); ok && token != "" {
				return token, nil
			}
		}
	}

	return "", errors.New("no access token found. Set CLAUDE_ACCESS_TOKEN or authenticate with Claude")
}

// getOrgUUID retrieves the organization UUID from config
func getOrgUUID() (string, error) {
	// Try environment variable first
	if orgUUID := os.Getenv("CLAUDE_ORG_UUID"); orgUUID != "" {
		return orgUUID, nil
	}

	// Try config file
	configPath := filepath.Join(os.Getenv("HOME"), ".claude.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", errors.New("could not read ~/.claude.json. Set CLAUDE_ORG_UUID or configure Claude")
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}

	if orgUUID, ok := config["org_uuid"].(string); ok && orgUUID != "" {
		return orgUUID, nil
	}

	// Check nested structure
	if orgs, ok := config["organizations"].([]interface{}); ok && len(orgs) > 0 {
		if org, ok := orgs[0].(map[string]interface{}); ok {
			if uuid, ok := org["uuid"].(string); ok {
				return uuid, nil
			}
		}
	}

	return "", errors.New("org_uuid not found in config. Set CLAUDE_ORG_UUID")
}

// getFromKeychain retrieves a password from macOS keychain
func getFromKeychain(server, account string) (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", server,
		"-a", account,
		"-w")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("keychain error: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
