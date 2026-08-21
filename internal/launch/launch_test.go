package launch

import (
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

func TestEnvNativeAnthropic(t *testing.T) {
	env, err := Env(config.Model{ID: "claude-opus-4-8", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("Env: %v", err)
	}
	if env["ANTHROPIC_MODEL"] != "claude-opus-4-8" {
		t.Errorf("ANTHROPIC_MODEL = %q", env["ANTHROPIC_MODEL"])
	}
	if _, ok := env["ANTHROPIC_BASE_URL"]; ok {
		t.Error("native mode must not set ANTHROPIC_BASE_URL")
	}
}

func TestEnvRejectsNonAnthropicInM1(t *testing.T) {
	if _, err := Env(config.Model{ID: "gpt-5.5", Provider: "zen"}); err == nil {
		t.Fatal("expected error for non-anthropic provider in M1")
	}
}

func TestCommand(t *testing.T) {
	if got := Command(); len(got) != 1 || got[0] != "claude" {
		t.Errorf("Command() = %v, want [claude]", got)
	}
}

func TestRoutedEnv(t *testing.T) {
	env, err := RoutedEnv(config.Model{ID: "gpt-5.5", Provider: "zen"}, 4000, "sk-master")
	if err != nil {
		t.Fatalf("RoutedEnv: %v", err)
	}
	if env["ANTHROPIC_BASE_URL"] != "http://localhost:4000" {
		t.Errorf("base url = %q", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-master" {
		t.Errorf("auth token = %q", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_MODEL"] != "gpt-5.5" {
		t.Errorf("model = %q", env["ANTHROPIC_MODEL"])
	}
}

func TestRoutedEnvRejectsNative(t *testing.T) {
	if _, err := RoutedEnv(config.Model{ID: "claude-opus-4-8", Provider: "anthropic"}, 4000, "k"); err == nil {
		t.Fatal("expected error routing a native anthropic model")
	}
}

// EnvKeys drives the clearing done on every launch, so anything Env or
// RoutedEnv can set must appear in it. A variable that drifts out of the list
// stops being cleared, and a routed session's copy of it survives into the next
// native one — the failure that made Anthropic sessions answer as GLM.
func TestEnvKeysCoversEverythingSet(t *testing.T) {
	keys := map[string]bool{}
	for _, k := range EnvKeys() {
		keys[k] = true
	}
	native, err := Env(config.Model{ID: "claude-opus-4-8", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("Env: %v", err)
	}
	routed, err := RoutedEnv(config.Model{ID: "gpt-5.5", Provider: "zen"}, 4000, "sk-master")
	if err != nil {
		t.Fatalf("RoutedEnv: %v", err)
	}
	for _, env := range []map[string]string{native, routed} {
		for k := range env {
			if !keys[k] {
				t.Errorf("%s is set on launch but missing from EnvKeys(), so it is never cleared", k)
			}
		}
	}
}
