package launch

import (
	"fmt"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

// RoutedEnv builds the env for a session that goes through the local LiteLLM
// proxy. It requires a non-anthropic (routed) model.
func RoutedEnv(m config.Model, port int, masterKey string) (map[string]string, error) {
	if !m.IsRouted() {
		return nil, fmt.Errorf("model %q is native anthropic; use Env, not RoutedEnv", m.ID)
	}
	return map[string]string{
		"ANTHROPIC_BASE_URL":   fmt.Sprintf("http://localhost:%d", port),
		"ANTHROPIC_AUTH_TOKEN": masterKey,
		"ANTHROPIC_MODEL":      m.ID,
	}, nil
}
