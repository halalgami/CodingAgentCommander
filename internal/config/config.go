// Package config loads the Commander model catalog from layered TOML.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Model is one selectable LLM in the catalog.
type Model struct {
	ID          string  `toml:"id"`                 // value passed as ANTHROPIC_MODEL
	Label       string  `toml:"label"`              // human display name
	Provider    string  `toml:"provider"`           // "anthropic" (native); "opencode-go"/"bedrock"/... routed
	InputPrice  float64 `toml:"input_price"`        // USD per 1M input tokens
	OutputPrice float64 `toml:"output_price"`       // USD per 1M output tokens
	KeyEnv      string  `toml:"key_env,omitempty"`  // env var LiteLLM reads: api_key: os.environ/<KeyEnv>
	Upstream    string  `toml:"upstream,omitempty"` // LiteLLM model string, e.g. "openai/gpt-5.5" or "bedrock/us.anthropic.claude-..."
	APIBase     string  `toml:"api_base,omitempty"` // upstream base URL for routed models
	Region      string  `toml:"region,omitempty"`   // AWS region for bedrock models (not a secret)
}

// Provider names. "anthropic" is native (subscription OAuth, no proxy); every
// other value is routed through the local LiteLLM proxy.
const (
	ProviderAnthropic = "anthropic"
	ProviderBedrock   = "bedrock"
)

// Well-known keychain refs shared by all AWS Bedrock models (one AWS account).
// The region is per-model config, not a secret, so it is not stored here.
// AWSSessionTokenEnv is optional — set only for temporary/STS credentials.
const (
	AWSAccessKeyEnv    = "AWS_ACCESS_KEY_ID"
	AWSSecretKeyEnv    = "AWS_SECRET_ACCESS_KEY"
	AWSSessionTokenEnv = "AWS_SESSION_TOKEN"
)

// CredEnvs returns the keychain refs a model REQUIRES before it can launch.
// Native anthropic needs none (subscription OAuth). Bedrock needs an AWS access
// key + secret. Every other routed provider needs its single KeyEnv.
func (m Model) CredEnvs() []string {
	switch m.Provider {
	case ProviderAnthropic:
		return nil
	case ProviderBedrock:
		return []string{AWSAccessKeyEnv, AWSSecretKeyEnv}
	default:
		if m.KeyEnv == "" {
			return nil
		}
		return []string{m.KeyEnv}
	}
}

// OptionalCredEnvs returns keychain refs a model can use if present but does not
// require. Bedrock accepts an AWS session token for temporary/STS credentials
// (SSO, assumed roles); its absence means long-lived keys.
func (m Model) OptionalCredEnvs() []string {
	if m.Provider == ProviderBedrock {
		return []string{AWSSessionTokenEnv}
	}
	return nil
}

// PreservesThinking reports whether a routed model should NOT have its
// extended-thinking blocks stripped by the proxy hook. Bedrock Claude speaks
// Anthropic thinking natively (via Converse), so stripping would needlessly
// disable its best coding mode. Non-anthropic open-weight upstreams still need
// the strip (see router.HookSource).
func (m Model) PreservesThinking() bool {
	return m.Provider == ProviderBedrock && strings.Contains(m.Upstream, "anthropic")
}

// NormalizeBedrockUpstream prepends the "bedrock/" LiteLLM provider prefix if the
// user omitted it, so "us.anthropic.claude-..." and "bedrock/us.anthropic..."
// both work.
func NormalizeBedrockUpstream(upstream string) string {
	if upstream != "" && !strings.HasPrefix(upstream, "bedrock/") {
		return "bedrock/" + upstream
	}
	return upstream
}

// Config is the resolved Commander configuration.
type Config struct {
	TmuxSession  string  `toml:"tmux_session"`
	DefaultModel string  `toml:"default_model"`
	Models       []Model `toml:"models"`
}

// IsRouted reports whether this model must go through the local LiteLLM proxy.
func (m Model) IsRouted() bool { return m.Provider != ProviderAnthropic }

// Model returns the catalog model with the given ID.
func (c Config) Model(id string) (Model, bool) {
	for _, m := range c.Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Default is the first-run starter catalog: native Anthropic models only, so
// the app is launchable with zero keys on a claude.ai subscription.
func Default() Config {
	return Config{
		TmuxSession:  "commander",
		DefaultModel: "claude-opus-4-8",
		Models: []Model{
			{ID: "claude-opus-4-8", Label: "Anthropic · Opus 4.8", Provider: ProviderAnthropic, InputPrice: 15, OutputPrice: 75},
			{ID: "claude-sonnet-5", Label: "Anthropic · Sonnet 5", Provider: ProviderAnthropic, InputPrice: 3, OutputPrice: 15},
		},
	}
}

// Load reads and validates a TOML config file.
func Load(path string) (Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if c.TmuxSession == "" {
		c.TmuxSession = "commander"
	}
	if len(c.Models) == 0 {
		return Config{}, fmt.Errorf("config %s: at least one [[models]] entry required", path)
	}
	if c.DefaultModel != "" {
		if _, ok := c.Model(c.DefaultModel); !ok {
			return Config{}, fmt.Errorf("config %s: default_model %q not found in catalog", path, c.DefaultModel)
		}
	}
	return c, nil
}

// Save writes the catalog to path as TOML (atomic temp+rename). Commander owns
// this file's contents — comments/formatting are not preserved.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := toml.NewEncoder(tmp).Encode(c); err != nil {
		tmp.Close()
		return fmt.Errorf("encode config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
