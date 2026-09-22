package delegate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/config"
)

func ollamaCatalog() []config.Model {
	return []config.Model{
		{ID: "ollama-glm-5.3", Provider: config.ProviderOllama,
			Upstream: "ollama_chat/glm-5.3", APIBase: "https://ollama.com", KeyEnv: "OLLAMA_API_KEY"},
		{ID: "claude-opus-5", Provider: config.ProviderAnthropic},
		{ID: "glm-5.2", Provider: config.ProviderOpencodeGo, Upstream: "openai/glm-5.2"},
	}
}

func TestWorkerModelName(t *testing.T) {
	if got := WorkerModelName("ollama-glm-5.3"); got != "ollama-glm-5.3-oai" {
		t.Errorf("got %q", got)
	}
}

func TestWorkerYAMLUsesUpstreamNameNotCatalogID(t *testing.T) {
	b, err := WorkerYAML(ollamaCatalog(), "sk-local")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// The catalog ID is "ollama-" prefixed; that name does not exist upstream
	// and every call would 404. The upstream name comes from Upstream.
	if !strings.Contains(s, "hosted_vllm/glm-5.3") {
		t.Errorf("want hosted_vllm/glm-5.3 in:\n%s", s)
	}
	if strings.Contains(s, "hosted_vllm/ollama-glm-5.3") {
		t.Error("catalog ID leaked into the upstream model name; every call would 404")
	}
	if !strings.Contains(s, "model_name: ollama-glm-5.3-oai") {
		t.Errorf("want the -oai model_name in:\n%s", s)
	}
}

func TestWorkerYAMLAppendsV1(t *testing.T) {
	b, _ := WorkerYAML(ollamaCatalog(), "sk-local")
	s := string(b)
	// config.OllamaDefaultAPIBase is the bare origin on purpose for the
	// ollama_chat path; the hosted_vllm path needs /v1.
	if !strings.Contains(s, "api_base: https://ollama.com/v1") {
		t.Errorf("want /v1 appended in:\n%s", s)
	}
}

func TestWorkerYAMLHasNoThinkField(t *testing.T) {
	b, _ := WorkerYAML(ollamaCatalog(), "sk-local")
	// think is an ollama_chat parameter; on the hosted_vllm path the reasoning
	// lever is output_config.effort, set per-run via --effort.
	if strings.Contains(string(b), "think:") {
		t.Error("think must not appear on a hosted_vllm entry")
	}
}

func TestWorkerYAMLReliabilityKnobs(t *testing.T) {
	b, _ := WorkerYAML(ollamaCatalog(), "sk-local")
	s := string(b)
	for _, want := range []string{"num_retries: 0", "request_timeout: 240", "drop_params: true", "strip_thinking.proxy_handler_instance", "master_key: sk-local"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestWorkerYAMLSkipsNonOllamaAndKeepsKeyAsEnvRef(t *testing.T) {
	b, _ := WorkerYAML(ollamaCatalog(), "sk-local")
	s := string(b)
	if strings.Contains(s, "claude-opus-5") || strings.Contains(s, "glm-5.2") {
		t.Error("only ollama-cloud models belong in the worker config")
	}
	if !strings.Contains(s, "api_key: os.environ/OLLAMA_API_KEY") {
		t.Error("the literal key must never land in the file")
	}
}

func TestWorkerYAMLRefusesEmptyCatalog(t *testing.T) {
	_, err := WorkerYAML([]config.Model{{ID: "x", Provider: config.ProviderAnthropic}}, "sk")
	if err == nil {
		t.Fatal("want an error when no ollama models exist")
	}
	if !strings.HasPrefix(err.Error(), "refusing to") {
		t.Errorf("error = %q, want a `refusing to` refusal", err)
	}
}

func TestWorkerYAMLSkipsWrongPrefixUpstream(t *testing.T) {
	// "ollama/glm-5.3" instead of "ollama_chat/glm-5.3": config.go carries a
	// named warning that the "ollama/" provider never sends the bearer token.
	// This class of mistake must be skipped here (and refused at preflight in
	// cmd/delegate), not turned into a bogus
	// "hosted_vllm/ollama/glm-5.3" model that 404s at the first worker turn.
	models := []config.Model{
		{ID: "ollama-glm-5.3", Provider: config.ProviderOllama,
			Upstream: "ollama/glm-5.3", APIBase: "https://ollama.com", KeyEnv: "OLLAMA_API_KEY"},
	}
	_, err := WorkerYAML(models, "sk-local")
	if err == nil {
		t.Fatal("want an error: the only model has the wrong provider prefix and must be skipped, leaving an empty model_list")
	}
	if !strings.HasPrefix(err.Error(), "refusing to") {
		t.Errorf("error = %q, want a `refusing to` refusal", err)
	}
}

func TestRunProxyDirIsUniqueUnderStateDir(t *testing.T) {
	a, err := runProxyDir()
	if err != nil {
		t.Fatal(err)
	}
	b, err := runProxyDir()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("want two calls to produce different paths, so two concurrent runs cannot collide")
	}
	want := filepath.Join(StateDir(), "proxy") + string(filepath.Separator)
	for _, p := range []string{a, b} {
		if !strings.HasPrefix(p, want) {
			t.Errorf("path %q not under %q", p, want)
		}
	}
}

func TestOllamaAPIBase(t *testing.T) {
	cases := []struct {
		name   string
		models []config.Model
		want   string
	}{
		{
			name:   "no ollama rows falls back to the default",
			models: []config.Model{{Provider: config.ProviderAnthropic, APIBase: "https://irrelevant.example"}},
			want:   config.OllamaDefaultAPIBase,
		},
		{
			name:   "ollama row with an empty api_base falls back to the default",
			models: []config.Model{{Provider: config.ProviderOllama, APIBase: ""}},
			want:   config.OllamaDefaultAPIBase,
		},
		{
			name:   "ollama row with an api_base returns it",
			models: []config.Model{{Provider: config.ProviderOllama, APIBase: "https://custom.example"}},
			want:   "https://custom.example",
		},
		{
			name: "a non-ollama row's api_base is never returned",
			models: []config.Model{
				{Provider: config.ProviderAnthropic, APIBase: "https://wrong.example"},
				{Provider: config.ProviderOllama, APIBase: "https://right.example"},
			},
			want: "https://right.example",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ollamaAPIBase(tc.models); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSweepStaleProxyDirsRemovesStaleDir(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "stale-run")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(stale, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("model_list: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * staleProxyDirAge)
	if err := os.Chtimes(cfgPath, old, old); err != nil {
		t.Fatal(err)
	}

	sweepStaleProxyDirs(root)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("want the stale directory removed, stat err = %v", err)
	}
}

func TestSweepStaleProxyDirsKeepsFreshDir(t *testing.T) {
	// A directory this young could belong to a currently running concurrent
	// sibling; the sweep must never destroy it. See staleProxyDirAge's doc for
	// why an age threshold, not a liveness check, is the guarantee this
	// package can actually make.
	root := t.TempDir()
	fresh := filepath.Join(root, "fresh-run")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(fresh, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("model_list: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sweepStaleProxyDirs(root)

	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("want the fresh directory kept, stat err = %v", err)
	}
}

func TestStaleProxyDirAgeExceedsRunBudget(t *testing.T) {
	// A live run's directory ages from the config.yaml write until Stop(), so
	// its worst case is the health loop plus Run's own deadline plus the wait
	// delay. If staleProxyDirAge ever fails to exceed that, sweepStaleProxyDirs
	// can reap a slow-but-healthy sibling mid-run — the mutual-kill bug the
	// per-run directories exist to prevent, reintroduced from the other side.
	boot := time.Duration(healthPollAttempts) * healthPollInterval
	var worst time.Duration
	var worstRole string
	for _, r := range Roles() {
		if b := boot + time.Duration(r.TimeoutSec)*time.Second + workerWaitDelay; b > worst {
			worst, worstRole = b, r.Name
		}
	}
	if staleProxyDirAge <= worst {
		t.Fatalf("staleProxyDirAge = %s must exceed the worst live-run budget %s (role %q); raise it or lower that role's TimeoutSec",
			staleProxyDirAge, worst, worstRole)
	}
}
