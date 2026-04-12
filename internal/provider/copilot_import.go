package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readCopilotTokenFromFile(path, source string) (string, error) {
	expanded := expandHome(path)
	data, err := os.ReadFile(expanded)
	if err != nil {
		return "", fmt.Errorf("%s: token file not found at %s", source, expanded)
	}

	switch source {
	case "neovim":
		return parseNeovimHosts(data)
	case "opencode":
		return parseOpencodeAuth(data)
	default:
		return "", fmt.Errorf("unknown source: %s", source)
	}
}

// parseNeovimHosts extracts the oauth_token from hosts.json.
// Format: {"github.com": {"oauth_token": "gho_..."}}
func parseNeovimHosts(data []byte) (string, error) {
	var hosts map[string]struct {
		OAuthToken string `json:"oauth_token"`
	}
	if err := json.Unmarshal(data, &hosts); err != nil {
		return "", fmt.Errorf("neovim: invalid hosts.json: %w", err)
	}
	for _, host := range hosts {
		if strings.HasPrefix(host.OAuthToken, "gho_") {
			return host.OAuthToken, nil
		}
	}
	return "", fmt.Errorf("neovim: no gho_ token found in hosts.json")
}

// parseOpencodeAuth extracts the access token from opencode's auth.json.
// Format: {"github-copilot": {"access": "gho_...", ...}}
func parseOpencodeAuth(data []byte) (string, error) {
	var entries map[string]struct {
		Access string `json:"access"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return "", fmt.Errorf("opencode: invalid auth.json: %w", err)
	}
	entry, ok := entries["github-copilot"]
	if !ok {
		return "", fmt.Errorf("opencode: no github-copilot entry in auth.json")
	}
	if !strings.HasPrefix(entry.Access, "gho_") {
		return "", fmt.Errorf("opencode: token is not a gho_ token")
	}
	return entry.Access, nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
