package stats

import "strings"

// ProviderOf maps a transcript model id to a provider: the configured catalog
// first, then prefix rules for models no longer configured. Unknown → "Other".
func ProviderOf(model string, catalog func(string) string) string {
	if catalog != nil {
		if p := catalog(model); p != "" {
			return p
		}
	}
	switch {
	case strings.HasPrefix(model, "anthropic.") || strings.Contains(model, ".anthropic."):
		return "bedrock"
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "glm-") || strings.HasPrefix(model, "Ollama · "):
		return "ollama-cloud"
	case strings.HasPrefix(model, "Go · "):
		return "opencode-go"
	}
	return "Other"
}
