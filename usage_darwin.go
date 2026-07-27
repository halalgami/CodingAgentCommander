//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

// claudeOAuthToken reads Claude Code's access token from the keychain item the
// CLI maintains ("Claude Code-credentials"). First access triggers a macOS
// keychain consent prompt for Commander — expected.
func claudeOAuthToken() (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return "", fmt.Errorf("Claude Code login not found in keychain (open claude once, or allow Commander keychain access)")
	}
	return parseClaudeCredential(string(out))
}
