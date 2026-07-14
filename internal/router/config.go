// Package router manages a local LiteLLM proxy for routed (non-Anthropic) models.
package router

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

// litellmModel maps a catalog upstream to the LiteLLM model string. Claude Code
// only speaks the Anthropic protocol, so LiteLLM bridges every request through
// its /v1/messages handler. For "openai/*" upstreams that bridge routes to the
// OpenAI *Responses* API (/responses), which OpenAI-compatible gateways like
// OpenCode Go/Zen don't serve (they serve /chat/completions) — yielding a 404.
// Rewriting to LiteLLM's "hosted_vllm/*" provider forces /chat/completions,
// which those gateways do serve. Users still write the natural "openai/<id>".
func litellmModel(upstream string) string {
	if rest, ok := strings.CutPrefix(upstream, "openai/"); ok {
		return "hosted_vllm/" + rest
	}
	return upstream
}

type llmParams struct {
	Model              string `yaml:"model"`
	APIBase            string `yaml:"api_base,omitempty"`
	APIKey             string `yaml:"api_key,omitempty"`
	AWSAccessKeyID     string `yaml:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `yaml:"aws_secret_access_key,omitempty"`
	AWSSessionToken    string `yaml:"aws_session_token,omitempty"`
	AWSRegionName      string `yaml:"aws_region_name,omitempty"`
}

// Options carries runtime-dependent bits GenerateConfig can't derive from the
// model catalog alone.
type Options struct {
	// AWSSessionToken wires aws_session_token from the env for bedrock models.
	// Set only when a token is actually present — an empty token breaks SigV4.
	AWSSessionToken bool
}

// SkipThinkingEnv names the env var the strip_thinking hook reads to learn which
// model_names must NOT have their thinking blocks stripped (comma-separated).
const SkipThinkingEnv = "STRIP_THINKING_SKIP"

// ThinkingSkipIDs returns the catalog IDs whose extended thinking the proxy hook
// must preserve (Bedrock Claude), for wiring into SkipThinkingEnv.
func ThinkingSkipIDs(models []config.Model) []string {
	var ids []string
	for _, m := range models {
		if m.IsRouted() && m.PreservesThinking() {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

type llmEntry struct {
	ModelName string    `yaml:"model_name"`
	Params    llmParams `yaml:"litellm_params"`
}

type llmConfig struct {
	ModelList       []llmEntry     `yaml:"model_list"`
	GeneralSettings map[string]any `yaml:"general_settings"`
	LitellmSettings map[string]any `yaml:"litellm_settings,omitempty"`
}

// HookModule/HookFile name the LiteLLM pre-call hook written next to the config.
// GenerateConfig references it as a callback; the controller runs litellm with
// its cwd set to the config dir so the module imports.
const (
	HookModule = "strip_thinking"
	HookFile   = HookModule + ".py"
)

// HookSource is a LiteLLM pre-call hook that makes OpenCode Go's open-weight
// models work through LiteLLM's Anthropic->OpenAI bridge. Two upstream breakages
// it repairs, both triggered by extended-thinking data Claude Code round-trips:
//   - Moonshot (kimi) rejects assistant message parts of type "thinking"
//     (LiteLLM manufactures them from reasoning_content on the way out, then
//     forwards them verbatim on the next turn) -> strip those parts.
//   - Console Go (deepseek) rejects the translated top-level thinking/reasoning
//     request params -> drop them (models still reason internally).
func HookSource() []byte {
	return []byte(`import os
from litellm.integrations.custom_logger import CustomLogger

_DROP_KEYS = ("thinking", "reasoning", "reasoning_effort")

# model_names that speak thinking natively (e.g. Bedrock Claude) — skip them so
# their extended-thinking mode isn't disabled by the strip below.
_SKIP = {s for s in os.environ.get("` + SkipThinkingEnv + `", "").split(",") if s}


def _clean(messages):
    for m in messages or []:
        c = m.get("content")
        if isinstance(c, list):
            m["content"] = [
                p for p in c
                if not (isinstance(p, dict) and p.get("type") in ("thinking", "redacted_thinking"))
            ]
    return messages


class StripThinking(CustomLogger):
    async def async_pre_call_hook(self, user_api_key_dict, cache, data, call_type):
        if data.get("model") in _SKIP:
            return data
        if "messages" in data:
            data["messages"] = _clean(data["messages"])
        for k in _DROP_KEYS:
            data.pop(k, None)
        return data


proxy_handler_instance = StripThinking()
`)
}

// GenerateConfig builds a LiteLLM config.yaml for the routed models in the
// catalog. Native (anthropic) models are excluded. Keys are referenced via
// os.environ/<KeyEnv> so the literal key never lands in the file.
func GenerateConfig(models []config.Model, masterKey string, opts Options) ([]byte, error) {
	cfg := llmConfig{
		GeneralSettings: map[string]any{"master_key": masterKey},
		// Claude Code sends Anthropic-only params (e.g. context_management) that
		// non-Anthropic upstreams reject; drop them instead of erroring. The
		// strip_thinking callback repairs thinking-block breakage the bridge
		// can't (see HookSource).
		LitellmSettings: map[string]any{
			"drop_params": true,
			"callbacks":   HookModule + ".proxy_handler_instance",
		},
	}
	for _, m := range models {
		if !m.IsRouted() {
			continue
		}
		p := llmParams{Model: litellmModel(m.Upstream)}
		if m.Provider == config.ProviderBedrock {
			// Bedrock authenticates with AWS SigV4, not a bearer key. LiteLLM
			// reads the credentials from these env refs and the region literal;
			// no api_base (the SDK derives the regional endpoint).
			p.AWSAccessKeyID = "os.environ/" + config.AWSAccessKeyEnv
			p.AWSSecretAccessKey = "os.environ/" + config.AWSSecretKeyEnv
			p.AWSRegionName = m.Region
			if opts.AWSSessionToken {
				p.AWSSessionToken = "os.environ/" + config.AWSSessionTokenEnv
			}
		} else {
			p.APIBase = m.APIBase
			p.APIKey = "os.environ/" + m.KeyEnv
		}
		cfg.ModelList = append(cfg.ModelList, llmEntry{ModelName: m.ID, Params: p})
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal litellm config: %w", err)
	}
	return out, nil
}
