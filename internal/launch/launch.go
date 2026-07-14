// Package launch builds the child-process environment for a Claude Code session.
package launch

import (
	"fmt"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

// Env returns the environment for launching a session on the given model.
// M1 supports native Anthropic only: it sets ANTHROPIC_MODEL and deliberately
// omits ANTHROPIC_BASE_URL so subscription OAuth keeps working.
func Env(m config.Model) (map[string]string, error) {
	if m.Provider != config.ProviderAnthropic {
		return nil, fmt.Errorf("provider %q is routed; use RoutedEnv, not Env", m.Provider)
	}
	return map[string]string{"ANTHROPIC_MODEL": m.ID}, nil
}

// Command is the program launched inside the tmux window.
func Command() []string { return []string{"claude"} }
