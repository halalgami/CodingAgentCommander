package router

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

func TestGenerateConfigRoutedOnly(t *testing.T) {
	models := []config.Model{
		{ID: "claude-opus-4-8", Provider: "anthropic"}, // native, excluded
		{ID: "gpt-5.5", Provider: "zen", Upstream: "openai/gpt-5.5", APIBase: "https://opencode.ai/zen/v1", KeyEnv: "ZEN_KEY"},
	}
	out, err := GenerateConfig(models, "sk-master", Options{})
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	var parsed struct {
		ModelList []struct {
			ModelName     string `yaml:"model_name"`
			LitellmParams struct {
				Model   string `yaml:"model"`
				APIBase string `yaml:"api_base"`
				APIKey  string `yaml:"api_key"`
			} `yaml:"litellm_params"`
		} `yaml:"model_list"`
		GeneralSettings struct {
			MasterKey string `yaml:"master_key"`
		} `yaml:"general_settings"`
		LitellmSettings struct {
			DropParams bool   `yaml:"drop_params"`
			Callbacks  string `yaml:"callbacks"`
		} `yaml:"litellm_settings"`
	}
	if err := yaml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.LitellmSettings.DropParams {
		t.Error("drop_params should be true")
	}
	if parsed.LitellmSettings.Callbacks != HookModule+".proxy_handler_instance" {
		t.Errorf("callbacks = %q, want the strip_thinking hook", parsed.LitellmSettings.Callbacks)
	}
	if len(parsed.ModelList) != 1 {
		t.Fatalf("expected 1 routed model, got %d", len(parsed.ModelList))
	}
	e := parsed.ModelList[0]
	// openai/* upstreams are rewritten to hosted_vllm/* so the anthropic->openai
	// bridge uses /chat/completions (not the Responses API the gateway lacks).
	if e.ModelName != "gpt-5.5" || e.LitellmParams.Model != "hosted_vllm/gpt-5.5" ||
		e.LitellmParams.APIBase != "https://opencode.ai/zen/v1" ||
		e.LitellmParams.APIKey != "os.environ/ZEN_KEY" {
		t.Errorf("entry wrong: %+v", e)
	}
	if parsed.GeneralSettings.MasterKey != "sk-master" {
		t.Errorf("master key = %q", parsed.GeneralSettings.MasterKey)
	}
}

func TestGenerateConfigBedrock(t *testing.T) {
	models := []config.Model{
		{ID: "claude-opus-4-8", Provider: "anthropic"}, // native, excluded
		{ID: "bedrock-sonnet", Provider: "bedrock",
			Upstream: "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0", Region: "us-east-1"},
	}
	out, err := GenerateConfig(models, "sk-master", Options{})
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	var parsed struct {
		ModelList []struct {
			ModelName     string `yaml:"model_name"`
			LitellmParams struct {
				Model              string `yaml:"model"`
				APIKey             string `yaml:"api_key"`
				APIBase            string `yaml:"api_base"`
				AWSAccessKeyID     string `yaml:"aws_access_key_id"`
				AWSSecretAccessKey string `yaml:"aws_secret_access_key"`
				AWSRegionName      string `yaml:"aws_region_name"`
			} `yaml:"litellm_params"`
		} `yaml:"model_list"`
	}
	if err := yaml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.ModelList) != 1 {
		t.Fatalf("expected 1 routed model, got %d", len(parsed.ModelList))
	}
	e := parsed.ModelList[0]
	if e.ModelName != "bedrock-sonnet" ||
		e.LitellmParams.Model != "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0" ||
		e.LitellmParams.AWSAccessKeyID != "os.environ/"+config.AWSAccessKeyEnv ||
		e.LitellmParams.AWSSecretAccessKey != "os.environ/"+config.AWSSecretKeyEnv ||
		e.LitellmParams.AWSRegionName != "us-east-1" {
		t.Errorf("bedrock entry wrong: %+v", e)
	}
	// Bedrock must NOT carry a bearer api_key or api_base.
	if e.LitellmParams.APIKey != "" || e.LitellmParams.APIBase != "" {
		t.Errorf("bedrock entry should have no api_key/api_base: %+v", e)
	}
}

func TestGenerateConfigBedrockSessionToken(t *testing.T) {
	models := []config.Model{
		{ID: "bedrock-sonnet", Provider: "bedrock",
			Upstream: "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0", Region: "us-east-1"},
	}
	// Without the option, no session token line.
	off, _ := GenerateConfig(models, "k", Options{})
	if strings.Contains(string(off), "aws_session_token") {
		t.Error("session token must be absent unless enabled (empty token breaks SigV4)")
	}
	// With it, the env ref is wired.
	on, _ := GenerateConfig(models, "k", Options{AWSSessionToken: true})
	if !strings.Contains(string(on), "aws_session_token: os.environ/"+config.AWSSessionTokenEnv) {
		t.Errorf("session token ref missing: %s", on)
	}
}

func TestThinkingSkipIDs(t *testing.T) {
	models := []config.Model{
		{ID: "bedrock-claude", Provider: "bedrock", Upstream: "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0"},
		{ID: "bedrock-nova", Provider: "bedrock", Upstream: "bedrock/us.amazon.nova-pro-v1:0"},
		{ID: "kimi", Provider: "zen", Upstream: "openai/kimi", KeyEnv: "ZEN_KEY"},
		{ID: "opus", Provider: "anthropic"},
	}
	got := ThinkingSkipIDs(models)
	if len(got) != 1 || got[0] != "bedrock-claude" {
		t.Errorf("ThinkingSkipIDs = %v, want [bedrock-claude] (only anthropic-on-bedrock)", got)
	}
}

func TestHookHonorsSkipEnv(t *testing.T) {
	src := string(HookSource())
	if !strings.Contains(src, SkipThinkingEnv) {
		t.Errorf("hook must read %s env", SkipThinkingEnv)
	}
	if !strings.Contains(src, `data.get("model") in _SKIP`) {
		t.Error("hook must short-circuit skipped model_names before stripping")
	}
}

func TestLitellmModelMapping(t *testing.T) {
	if got := litellmModel("openai/glm-5.2"); got != "hosted_vllm/glm-5.2" {
		t.Errorf("openai/ not rewritten: %q", got)
	}
	if got := litellmModel("anthropic/qwen3.6-plus"); got != "anthropic/qwen3.6-plus" {
		t.Errorf("anthropic/ must pass through: %q", got)
	}
}

func TestHookSource(t *testing.T) {
	src := string(HookSource())
	for _, want := range []string{"proxy_handler_instance", "async_pre_call_hook", "thinking", "reasoning_effort"} {
		if !strings.Contains(src, want) {
			t.Errorf("hook source missing %q", want)
		}
	}
	if HookFile != "strip_thinking.py" {
		t.Errorf("HookFile = %q", HookFile)
	}
}
