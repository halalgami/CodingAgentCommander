// Package launch builds the child-process environment for a Claude Code session.
package launch

import (
	"fmt"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

// The environment variables Commander injects into a session. Named here so
// EnvKeys can enumerate them and Env/RoutedEnv can't drift from that list.
const (
	EnvModel     = "ANTHROPIC_MODEL"
	EnvBaseURL   = "ANTHROPIC_BASE_URL"
	EnvAuthToken = "ANTHROPIC_AUTH_TOKEN"
)

// EnvKeys is every variable Commander may set on a session, whichever provider
// it launches. A launch clears all of them before applying the ones it needs,
// so a routed session's proxy vars can't survive into a later native one — see
// tmux.LaunchSpec.ClearEnv.
func EnvKeys() []string { return []string{EnvModel, EnvBaseURL, EnvAuthToken} }

// Env returns the environment for launching a session on the given model.
// Native Anthropic sets ANTHROPIC_MODEL and deliberately omits
// ANTHROPIC_BASE_URL so subscription OAuth keeps working. Omitting is not
// enough on its own — a base URL left behind by an earlier routed launch would
// still be inherited — which is what ClearEnv exists to prevent.
func Env(m config.Model) (map[string]string, error) {
	if m.Provider != config.ProviderAnthropic {
		return nil, fmt.Errorf("provider %q is routed; use RoutedEnv, not Env", m.Provider)
	}
	return map[string]string{EnvModel: m.ID}, nil
}

// Command is the program launched inside the tmux window.
func Command() []string { return []string{"claude"} }
