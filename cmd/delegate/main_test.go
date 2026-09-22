package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const miniConfig = `tmux_session = "commander"
default_model = "claude-opus-5"

[[models]]
id = "claude-opus-5"
label = "Anthropic · Opus 5"
provider = "anthropic"
input_price = 5.0
output_price = 25.0

[[models]]
id = "ollama-glm-5.3"
label = "Ollama · glm-5.3"
provider = "ollama-cloud"
upstream = "ollama_chat/glm-5.3"
input_price = 0.0
output_price = 0.0
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSplitPaths(t *testing.T) {
	got := splitPaths(" internal/router , app.go ,, ")
	if len(got) != 2 || got[0] != "internal/router" || got[1] != "app.go" {
		t.Errorf("got %#v", got)
	}
	if splitPaths("   ") != nil {
		t.Error("blank input must yield nil, not an empty flag")
	}
}

func TestRunRefusesEmptyTask(t *testing.T) {
	_, err := run(context.Background(), options{Role: "scout", Task: "  ", Root: ".", Config: writeConfig(t, miniConfig)})
	if err == nil || !strings.HasPrefix(err.Error(), "refusing to") {
		t.Fatalf("want a `refusing to` refusal, got %v", err)
	}
}

func TestRunRefusesUnknownRole(t *testing.T) {
	_, err := run(context.Background(), options{Role: "nope", Task: "do it", Root: ".", Config: writeConfig(t, miniConfig)})
	if err == nil || !strings.Contains(err.Error(), "scout") {
		t.Fatalf("want an error listing valid roles, got %v", err)
	}
}

func TestRunRefusesModelMissingFromCatalog(t *testing.T) {
	// Ollama retires cloud models on a scale of weeks, so a shipped role can
	// name a model the catalog no longer has. That must fail before a proxy
	// starts, not at turn time.
	noOllama := strings.SplitN(miniConfig, "[[models]]\nid = \"ollama-glm-5.3\"", 2)[0]
	_, err := run(context.Background(), options{Role: "scout", Task: "do it", Root: ".", Config: writeConfig(t, noOllama)})
	if err == nil || !strings.Contains(err.Error(), "not in the catalog") {
		t.Fatalf("want a catalog refusal, got %v", err)
	}
}

func TestRunRefusesModelWithUnusableUpstream(t *testing.T) {
	// A catalog row can match on ID but still be unusable to the worker
	// proxy: WorkerYAML (proxy.go) independently skips a row whose provider
	// isn't ollama-cloud or whose upstream is empty/unprefixed. Preflight
	// must catch that too, not just a missing ID, or the proxy starts
	// healthy and every worker turn 400s on an unknown model.
	badUpstream := strings.Replace(miniConfig, `upstream = "ollama_chat/glm-5.3"`, `upstream = ""`, 1)
	_, err := run(context.Background(), options{Role: "scout", Task: "do it", Root: ".", Config: writeConfig(t, badUpstream)})
	if err == nil || !strings.Contains(err.Error(), "not in the catalog") {
		t.Fatalf("want a catalog refusal, got %v", err)
	}
}

func TestRunRefusesModelWithWrongPrefixUpstream(t *testing.T) {
	// "ollama/glm-5.3" instead of "ollama_chat/glm-5.3": config.go carries a
	// named warning that the "ollama/" provider never sends the bearer token.
	// WorkerYAML (proxy.go) skips a row like this too, but silently — leaving
	// an empty model_list only if no other ollama row exists. Preflight must
	// catch the wrong prefix on its own, not rely on the generator's skip
	// happening to also empty the catalog, or this class of mistake reaches
	// the worker and 404s at the first turn instead of failing here.
	wrongPrefix := strings.Replace(miniConfig, `upstream = "ollama_chat/glm-5.3"`, `upstream = "ollama/glm-5.3"`, 1)
	_, err := run(context.Background(), options{Role: "scout", Task: "do it", Root: ".", Config: writeConfig(t, wrongPrefix)})
	if err == nil || !strings.Contains(err.Error(), "not in the catalog") {
		t.Fatalf("want a catalog refusal, got %v", err)
	}
}

func TestRunRefusesMissingRoot(t *testing.T) {
	// A typo in -root should fail immediately, not ~8s later as failed_infra
	// after paying for a full proxy boot.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := run(context.Background(), options{Role: "scout", Task: "do it", Root: missing, Config: writeConfig(t, miniConfig)})
	if err == nil || !strings.HasPrefix(err.Error(), "refusing to") {
		t.Fatalf("want a `refusing to` refusal, got %v", err)
	}
}
