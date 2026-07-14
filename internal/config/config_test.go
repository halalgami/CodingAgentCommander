package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValidConfig(t *testing.T) {
	p := writeTemp(t, `
default_model = "claude-opus-4-8"

[[models]]
id = "claude-opus-4-8"
label = "Opus 4.8"
provider = "anthropic"
input_price = 15.0
output_price = 75.0

[[models]]
id = "claude-sonnet-5"
label = "Sonnet 5"
provider = "anthropic"
input_price = 3.0
output_price = 15.0
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.TmuxSession != "commander" {
		t.Errorf("TmuxSession default = %q, want commander", c.TmuxSession)
	}
	if len(c.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(c.Models))
	}
	m, ok := c.Model("opus")
	if ok {
		t.Errorf("Model(\"opus\") matched by label-key unexpectedly")
	}
	m, ok = c.Model("claude-opus-4-8")
	if !ok || m.Label != "Opus 4.8" {
		t.Errorf("Model lookup failed: %+v ok=%v", m, ok)
	}
}

func TestLoadRejectsUnknownDefault(t *testing.T) {
	p := writeTemp(t, `
default_model = "ghost"
[[models]]
id = "claude-opus-4-8"
label = "Opus 4.8"
provider = "anthropic"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for default_model not in catalog")
	}
}

func TestLoadRejectsEmptyCatalog(t *testing.T) {
	p := writeTemp(t, `default_model = "x"`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for empty models")
	}
}

func TestRoutedModelFields(t *testing.T) {
	p := writeTemp(t, `
default_model = "claude-opus-4-8"

[[models]]
id = "claude-opus-4-8"
label = "Opus"
provider = "anthropic"

[[models]]
id = "gpt-5.5"
label = "Zen GPT-5.5"
provider = "zen"
upstream = "openai/gpt-5.5"
api_base = "https://opencode.ai/zen/v1"
key_env = "ZEN_KEY"
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	anth, _ := c.Model("claude-opus-4-8")
	if anth.IsRouted() {
		t.Error("anthropic model must not be routed")
	}
	zen, ok := c.Model("gpt-5.5")
	if !ok || !zen.IsRouted() {
		t.Fatalf("zen model missing or not routed: %+v ok=%v", zen, ok)
	}
	if zen.Upstream != "openai/gpt-5.5" || zen.APIBase != "https://opencode.ai/zen/v1" || zen.KeyEnv != "ZEN_KEY" {
		t.Errorf("routed fields wrong: %+v", zen)
	}
}

func TestCredEnvs(t *testing.T) {
	cases := []struct {
		name string
		m    Model
		want []string
	}{
		{"anthropic", Model{Provider: "anthropic"}, nil},
		{"zen", Model{Provider: "zen", KeyEnv: "ZEN_KEY"}, []string{"ZEN_KEY"}},
		{"bedrock", Model{Provider: "bedrock"}, []string{AWSAccessKeyEnv, AWSSecretKeyEnv}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.m.CredEnvs()
			if len(got) != len(tc.want) {
				t.Fatalf("CredEnvs = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("CredEnvs[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNormalizeBedrockUpstream(t *testing.T) {
	cases := map[string]string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0":         "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0",
		"bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0": "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0",
		"": "",
	}
	for in, want := range cases {
		if got := NormalizeBedrockUpstream(in); got != want {
			t.Errorf("NormalizeBedrockUpstream(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPreservesThinking(t *testing.T) {
	claude := Model{Provider: "bedrock", Upstream: "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0"}
	if !claude.PreservesThinking() {
		t.Error("bedrock claude should preserve thinking")
	}
	nova := Model{Provider: "bedrock", Upstream: "bedrock/us.amazon.nova-pro-v1:0"}
	if nova.PreservesThinking() {
		t.Error("bedrock nova is not anthropic; must not preserve thinking")
	}
	zen := Model{Provider: "zen", Upstream: "openai/kimi"}
	if zen.PreservesThinking() {
		t.Error("zen must not preserve thinking")
	}
}

func TestOptionalCredEnvs(t *testing.T) {
	if got := (Model{Provider: "bedrock"}).OptionalCredEnvs(); len(got) != 1 || got[0] != AWSSessionTokenEnv {
		t.Errorf("bedrock OptionalCredEnvs = %v, want [%s]", got, AWSSessionTokenEnv)
	}
	if got := (Model{Provider: "zen", KeyEnv: "ZEN_KEY"}).OptionalCredEnvs(); got != nil {
		t.Errorf("zen OptionalCredEnvs = %v, want nil", got)
	}
}

func TestBedrockModelFields(t *testing.T) {
	p := writeTemp(t, `
default_model = "claude-opus-4-8"

[[models]]
id = "claude-opus-4-8"
label = "Opus"
provider = "anthropic"

[[models]]
id = "bedrock-sonnet"
label = "Bedrock Sonnet"
provider = "bedrock"
upstream = "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0"
region = "us-east-1"
`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m, ok := c.Model("bedrock-sonnet")
	if !ok || !m.IsRouted() {
		t.Fatalf("bedrock model missing or not routed: %+v ok=%v", m, ok)
	}
	if m.Region != "us-east-1" || m.Upstream != "bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0" {
		t.Errorf("bedrock fields wrong: %+v", m)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	c := Config{
		TmuxSession:  "commander",
		DefaultModel: "claude-opus-4-8",
		Models: []Model{
			{ID: "claude-opus-4-8", Label: "Opus", Provider: "anthropic", InputPrice: 15, OutputPrice: 75},
			{ID: "kimi", Label: "Kimi", Provider: "zen", Upstream: "openai/kimi", APIBase: "https://x/v1", KeyEnv: "ZEN_KEY"},
		},
	}
	p := filepath.Join(t.TempDir(), "out.toml")
	if err := Save(p, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.TmuxSession != "commander" || got.DefaultModel != "claude-opus-4-8" || len(got.Models) != 2 {
		t.Fatalf("top-level/round-trip mismatch: %+v", got)
	}
	kimi, ok := got.Model("kimi")
	if !ok || kimi.Upstream != "openai/kimi" || kimi.KeyEnv != "ZEN_KEY" || !kimi.IsRouted() {
		t.Errorf("routed model round-trip wrong: %+v", kimi)
	}
	// native model must not carry empty routed keys after round-trip
	opus, _ := got.Model("claude-opus-4-8")
	if opus.Upstream != "" || opus.KeyEnv != "" {
		t.Errorf("native model gained routed fields: %+v", opus)
	}
}
