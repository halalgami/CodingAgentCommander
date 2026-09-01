// Package config loads the Commander model catalog from layered TOML.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/halalgami/CodingAgentCommander/internal/anthropic"
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

// ProviderOpencodeGo is the OpenCode Zen/Go gateway (OpenAI-compatible, routed).
const ProviderOpencodeGo = "opencode-go"

// ProviderOllama is Ollama Cloud (routed through LiteLLM's ollama_chat provider).
const ProviderOllama = "ollama-cloud"

// Type-level credential constants for definable providers. The key itself
// lives in the OS keychain under this ref; only endpoint/region metadata is
// stored in TOML.
const (
	ZenKeyEnv         = "ZEN_KEY"
	ZenDefaultAPIBase = "https://opencode.ai/zen/go/v1"

	OllamaKeyEnv = "OLLAMA_API_KEY"
	// OllamaDefaultAPIBase is the BARE ORIGIN on purpose. LiteLLM's ollama_chat
	// provider builds "{api_base}/api/chat", so a "/v1" or "/api" suffix here
	// yields https://ollama.com/v1/api/chat — a 404 at the first turn.
	OllamaDefaultAPIBase = "https://ollama.com"
	// OllamaUpstreamPrefix is LiteLLM's provider prefix. Not "ollama/": that
	// provider's validate_environment is a stub that never sends the bearer
	// token, so it cannot reach the cloud host at all.
	OllamaUpstreamPrefix = "ollama_chat/"
)

// Provider is a user-defined upstream a model can route through. Anthropic is
// built-in (subscription OAuth) and never appears here.
type Provider struct {
	Type    string `toml:"type"`               // ProviderOpencodeGo | ProviderBedrock | ProviderOllama
	APIBase string `toml:"api_base,omitempty"` // opencode-go endpoint
	Region  string `toml:"region,omitempty"`   // bedrock default region
}

// DefinableProviderTypes are the provider types a user may add.
var DefinableProviderTypes = []string{ProviderOpencodeGo, ProviderBedrock, ProviderOllama}

// ProviderByType returns the defined provider entry of the given type.
func (c Config) ProviderByType(t string) (Provider, bool) {
	for _, p := range c.Providers {
		if p.Type == t {
			return p, true
		}
	}
	return Provider{}, false
}

// MigrateProviders synthesizes [[providers]] entries from legacy configs whose
// models carry inline api_base/key_env/region. First matching model wins.
// Returns true if the config changed (caller may persist).
func (c *Config) MigrateProviders() bool {
	changed := false
	for _, m := range c.Models {
		switch m.Provider {
		case ProviderOpencodeGo:
			if _, ok := c.ProviderByType(ProviderOpencodeGo); !ok {
				base := m.APIBase
				if base == "" {
					base = ZenDefaultAPIBase
				}
				c.Providers = append(c.Providers, Provider{Type: ProviderOpencodeGo, APIBase: base})
				changed = true
			}
		case ProviderBedrock:
			if _, ok := c.ProviderByType(ProviderBedrock); !ok {
				c.Providers = append(c.Providers, Provider{Type: ProviderBedrock, Region: m.Region})
				changed = true
			}
		}
	}
	return changed
}

// ResolveModel fills a model's empty provider-supplied fields (KeyEnv, APIBase,
// Region) from its provider entry. Inline values from legacy configs win.
// Launch/router/creds code paths consume resolved models only.
func (c Config) ResolveModel(m Model) Model {
	switch m.Provider {
	case ProviderOpencodeGo:
		p, ok := c.ProviderByType(ProviderOpencodeGo)
		if m.APIBase == "" && ok {
			m.APIBase = p.APIBase
		}
		if m.KeyEnv == "" {
			m.KeyEnv = ZenKeyEnv
		}
	case ProviderBedrock:
		if p, ok := c.ProviderByType(ProviderBedrock); ok && m.Region == "" {
			m.Region = p.Region
		}
	case ProviderOllama:
		if m.APIBase == "" {
			if p, ok := c.ProviderByType(ProviderOllama); ok {
				m.APIBase = p.APIBase
			}
		}
		if m.APIBase == "" {
			m.APIBase = OllamaDefaultAPIBase
		}
		if m.KeyEnv == "" {
			m.KeyEnv = OllamaKeyEnv
		}
	}
	return m
}

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

// BandByContext reports whether a session's meter should show context fullness
// rather than dollars per turn.
//
// True for providers that bill by subscription, where per-turn cost is not the
// scarce resource: native Anthropic (Claude Code's own subscription) and Ollama
// Cloud (a monthly plan with 5-hour and weekly session limits, and no published
// per-token rate). The name describes which meter to draw rather than asserting
// a billing model the code cannot know — an Anthropic user with an API key does
// pay per token.
func (m Model) BandByContext() bool {
	return m.Provider == ProviderAnthropic || m.Provider == ProviderOllama
}

// Unpriced reports that the catalog carries no rate for this model, so no dollar
// figure can honestly be shown. Discovery adds models at price 0 whenever the
// upstream does not report pricing.
func (m Model) Unpriced() bool { return m.InputPrice == 0 && m.OutputPrice == 0 }

// NormalizeBedrockUpstream prepends the "bedrock/" LiteLLM provider prefix if the
// user omitted it, so "us.anthropic.claude-..." and "bedrock/us.anthropic..."
// both work.
func NormalizeBedrockUpstream(upstream string) string {
	if upstream != "" && !strings.HasPrefix(upstream, "bedrock/") {
		return "bedrock/" + upstream
	}
	return upstream
}

// NormalizeOllamaUpstream prepends the "ollama_chat/" LiteLLM provider prefix if
// the user omitted it, so "glm-5.3" and "ollama_chat/glm-5.3" both work.
func NormalizeOllamaUpstream(upstream string) string {
	if upstream != "" && !strings.HasPrefix(upstream, OllamaUpstreamPrefix) {
		return OllamaUpstreamPrefix + upstream
	}
	return upstream
}

// OllamaCatalogID derives the catalog ID for an Ollama upstream: "ollama-" plus
// the model name, colons and dots preserved.
//
// The prefix exists because catalog IDs must be unique and Ollama and OpenCode
// Zen both serve models named glm-5.2. It is a hyphen rather than a slash
// because the ID is used both as ANTHROPIC_MODEL and as LiteLLM's model_name,
// where a slash parses as a provider prefix.
//
// This is the single source of truth: the Models drawer derives IDs by mangling
// dots and colons to hyphens, which would mint a second catalog row for a model
// added manually rather than discovered. AddModel calls this instead.
func OllamaCatalogID(upstream string) string {
	return "ollama-" + strings.TrimPrefix(upstream, OllamaUpstreamPrefix)
}

// Config is the resolved Commander configuration.
type Config struct {
	TmuxSession  string `toml:"tmux_session"`
	DefaultModel string `toml:"default_model"`
	// AnthropicCatalogRev is the anthropic.CatalogRev this config was last
	// merged against. Absent (0) on configs written before catalog merging
	// existed, which is what makes them pick it up on first launch.
	//
	// Declared before Models and Providers on purpose: the encoder emits fields
	// in declaration order, and a bare key written after an array of tables
	// belongs to that table in TOML, not to the document. Emitted last, this
	// would decode into a Model and be lost — leaving the revision at 0 so every
	// launch re-merged and rewrote the file, resurrecting deleted models.
	AnthropicCatalogRev int        `toml:"anthropic_catalog_rev,omitempty"`
	Models              []Model    `toml:"models"`
	Providers           []Provider `toml:"providers,omitempty"`
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
// the app is launchable with zero keys on a claude.ai subscription. Built from
// the anthropic package's catalog rather than a second hand-written list, which
// is how the picker came to be two releases behind.
func Default() Config {
	return Config{
		TmuxSession:         "commander",
		DefaultModel:        anthropic.DefaultID,
		Models:              AnthropicModels(),
		AnthropicCatalogRev: anthropic.CatalogRev,
	}
}

// AnthropicModels is the built-in native catalog as catalog entries.
func AnthropicModels() []Model {
	cat := anthropic.Catalog()
	out := make([]Model, 0, len(cat))
	for _, m := range cat {
		out = append(out, Model{
			ID: m.ID, Label: m.Label, Provider: ProviderAnthropic,
			InputPrice: m.InputPrice, OutputPrice: m.OutputPrice,
		})
	}
	return out
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
	seenProv := map[string]bool{}
	for _, p := range c.Providers {
		if !slices.Contains(DefinableProviderTypes, p.Type) {
			return Config{}, fmt.Errorf("config %s: unknown provider type %q", path, p.Type)
		}
		if seenProv[p.Type] {
			return Config{}, fmt.Errorf("config %s: duplicate provider type %q", path, p.Type)
		}
		seenProv[p.Type] = true
	}
	c.MigrateProviders()
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
