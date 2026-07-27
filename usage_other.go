//go:build !darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// claudeOAuthToken reads Claude Code's access token from the credential file the
// CLI maintains outside macOS: ~/.claude/.credentials.json. It holds the same
// JSON shape as the macOS keychain blob (a claudeAiOauth object).
func claudeOAuthToken() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return "", fmt.Errorf("Claude Code login not found — run `claude` once to sign in: %w", err)
	}
	return parseClaudeCredential(string(data))
}
