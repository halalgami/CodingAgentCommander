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
	// EnvMaxRetries bounds Claude Code's own retry ladder. Set on routed
	// sessions only — see RoutedMaxRetries.
	EnvMaxRetries = "CLAUDE_CODE_MAX_RETRIES"
)

// RoutedMaxRetries bounds retries for a routed session.
//
// Measured: a rejected API key took 244 seconds to surface and an upstream 500
// took 249 seconds — one turn, zero tokens, no output. The retrier is CLAUDE
// CODE, not LiteLLM (proven against a fake endpoint returning a constant 500
// with no proxy in the path at all: 9 requests over 111s and still climbing),
// so LiteLLM's own num_retries never controlled it. Without this, a stale key
// does not fail a session — it hangs it for four minutes with no explanation.
//
// Two rather than zero: a routed interactive session should survive one
// transient upstream blip rather than losing the user's turn to it. Delegate
// workers (internal/delegate) use 0 instead, because a delegated task should
// fail fast and report rather than making its caller wait.
//
// Native Anthropic is deliberately left alone: retrying a genuine overload is
// worth the wait there, and it is not the path that produced these numbers.
const RoutedMaxRetries = "2"

// EnvKeys is every variable Commander may set on a session, whichever provider
// it launches. A launch clears all of them before applying the ones it needs,
// so a routed session's proxy vars can't survive into a later native one — see
// tmux.LaunchSpec.ClearEnv.
func EnvKeys() []string {
	return []string{EnvModel, EnvBaseURL, EnvAuthToken, EnvMaxRetries}
}

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
