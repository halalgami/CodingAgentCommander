package delegate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/ollama"
	"github.com/halalgami/CodingAgentCommander/internal/router"
	"github.com/halalgami/CodingAgentCommander/internal/secrets"
)

// ProxyRequestTimeout is LiteLLM's per-request ceiling. It sits BELOW every
// role's TimeoutSec so an upstream stall surfaces as an upstream error rather
// than as our own kill — the reverse ordering makes request_timeout dead config.
const ProxyRequestTimeout = 240

// WorkerModelName derives the LiteLLM model_name for a worker entry. Mechanical,
// so nothing depends on user naming.
func WorkerModelName(catalogID string) string { return catalogID + "-oai" }

type workerParams struct {
	Model   string `yaml:"model"`
	APIBase string `yaml:"api_base"`
	APIKey  string `yaml:"api_key"`
}

type workerEntry struct {
	ModelName string       `yaml:"model_name"`
	Params    workerParams `yaml:"litellm_params"`
}

type workerConfig struct {
	ModelList       []workerEntry  `yaml:"model_list"`
	GeneralSettings map[string]any `yaml:"general_settings"`
	LitellmSettings map[string]any `yaml:"litellm_settings"`
}

// WorkerYAML generates a LiteLLM config for workers only.
//
// This deliberately does NOT call router.GenerateConfig: that function serves
// live interactive sessions, and its ollama branch pins think:false for the
// ollama_chat path. Workers use the OpenAI-compatible /v1 path instead, which
// is faster, fixes kimi's 500s and stops glm concatenating its reasoning into
// content (spec §4.2). Generating our own keeps the interactive path untouched.
func WorkerYAML(models []config.Model, masterKey string) ([]byte, error) {
	cfg := workerConfig{
		GeneralSettings: map[string]any{"master_key": masterKey},
		LitellmSettings: map[string]any{
			"drop_params": true,
			"callbacks":   router.HookModule + ".proxy_handler_instance",
			// Claude Code is the main retrier (spec §4.7), but leave nothing
			// on this side adding to it either.
			"num_retries":     0,
			"request_timeout": ProxyRequestTimeout,
		},
	}
	for _, m := range models {
		if m.Provider != config.ProviderOllama {
			continue
		}
		// Require the actual "ollama_chat/" prefix, not just a non-empty
		// Upstream: config.go carries a named warning that the "ollama/"
		// provider never sends the bearer token, so a row misconfigured as
		// "ollama/glm-5.3" must be skipped here rather than trimmed into a
		// bogus "hosted_vllm/ollama/glm-5.3" that 404s at the first worker
		// turn. This class of mistake must fail at preflight, not at turn time
		// (see main.go's matching check).
		//
		// Normalizing first was considered and REJECTED: NormalizeOllamaUpstream
		// blindly prepends when the prefix is absent, so "ollama/glm-5.3"
		// becomes "ollama_chat/ollama/glm-5.3" and would pass this very check.
		// The cost is that a BARE "glm-5.3" is also refused, which would have
		// worked here — but that form is reachable only from a hand-edited
		// config (SaveModel and discovery both normalize on write) and is
		// already unusable on the interactive path, so an actionable refusal
		// beats guessing what the operator meant.
		if !strings.HasPrefix(m.Upstream, config.OllamaUpstreamPrefix) {
			continue
		}
		// The upstream name, not the catalog ID: OllamaCatalogID prefixes
		// "ollama-", and "ollama-glm-5.3" does not exist at ollama.com.
		upstream := strings.TrimPrefix(m.Upstream, config.OllamaUpstreamPrefix)
		if upstream == "" {
			continue
		}
		base := m.APIBase
		if base == "" {
			base = config.OllamaDefaultAPIBase
		}
		keyEnv := m.KeyEnv
		if keyEnv == "" {
			keyEnv = config.OllamaKeyEnv
		}
		cfg.ModelList = append(cfg.ModelList, workerEntry{
			ModelName: WorkerModelName(m.ID),
			Params: workerParams{
				Model: "hosted_vllm/" + upstream,
				// The ollama_chat path wants the bare origin; hosted_vllm
				// speaks /chat/completions under /v1.
				APIBase: strings.TrimSuffix(base, "/") + "/v1",
				APIKey:  "os.environ/" + keyEnv,
			},
		})
	}
	if len(cfg.ModelList) == 0 {
		return nil, errors.New("refusing to start a proxy with no ollama-cloud models in the catalog; add one in Commander's Models panel")
	}
	return yaml.Marshal(cfg)
}

// Proxy is a LiteLLM process this binary owns for the life of one command.
//
// It does not try to reuse Commander's proxy: router.NewController(0) picks a
// free port at Start and holds it only in memory, so no sibling process can
// discover it, and ensureRouter restarts it whenever the config hash drifts —
// which would kill in-flight workers. Phase 2's in-GUI server removes this.
type Proxy struct {
	ctl *router.Controller
	dir string // per-run directory under StateDir()/proxy, removed on Stop
}

// runProxyDir returns a fresh, unique directory under StateDir()/proxy for one
// run's config.
//
// A fixed directory shared by every run meant a second concurrent delegate
// run overwrote the first's config.yaml (a different master key) and then, via
// router.ReapStale's `pkill -f <configPath>` on that same shared path, killed
// the first run's live LiteLLM out from under it — connection-refused for the
// rest of its deadline. Parallel recon is the natural shape of this tool, so
// each run gets its own path. That means no *other* process is ever started
// against this run's own cfgPath, so a ReapStale call on it can never match
// anything — cleanup of a run that never got to call Stop (crash, SIGKILL,
// power loss) is handled separately by sweepStaleProxyDirs, against every
// *other* directory already on disk.
func runProxyDir() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate proxy dir suffix: %w", err)
	}
	name := time.Now().UTC().Format("20060102-150405.000000") + "-" + hex.EncodeToString(buf)
	return filepath.Join(StateDir(), "proxy", name), nil
}

// staleProxyDirAge is the age past which a leftover proxy/<run> directory is
// treated as abandoned rather than a live sibling.
//
// There is no pid or liveness check available here (this package does not
// enumerate processes), so a directory cannot be proven dead — only "old
// enough to be implausible as a live run".
//
// The margin is NOT generous, and an earlier version of this comment claimed
// it was. A live directory's age is measured from the config.yaml write, so
// the worst case is the health loop (120 × 500ms = 60s) plus Run's own budget
// (TimeoutSec = 300s, plus 2s WaitDelay): about 6m02s against a 10m cutoff, a
// 1.66× margin. It is crossed as soon as TimeoutSec exceeds ~538s, which a
// future role tuning could plausibly do. TestStaleProxyDirAgeExceedsRunBudget
// asserts the relationship so it breaks loudly instead of silently.
//
// Chosen deliberately over reaping-by-default: destroying a live sibling's
// directory out from under it would resurrect exactly the bug this package's
// per-run dirs exist to fix, so this errs toward leaking a dead run's files
// for a while over ever killing a live one. If the threshold were ever
// crossed, the victim fails honestly — its worker gets connection-refused,
// CLAUDE_CODE_MAX_RETRIES=0 makes that fast, and it reports failed_infra.
const staleProxyDirAge = 10 * time.Minute

// healthPollAttempts and healthPollInterval bound how long StartProxy waits for
// litellm to answer /health/liveliness. Named because they are half of the
// worst-case age of a LIVE run directory, which staleProxyDirAge must exceed —
// TestStaleProxyDirAgeExceedsRunBudget asserts that relationship.
const (
	healthPollAttempts = 120
	healthPollInterval = 500 * time.Millisecond
)

// sweepStaleProxyDirs best-effort reaps every abandoned run directory under
// proxyRoot: for each entry older than staleProxyDirAge, it kills any orphan
// LiteLLM still holding that entry's config.yaml (router.ReapStale) and then
// removes the directory. It runs once, before the current run creates its own
// directory, so "every pre-existing entry" naturally excludes this run.
//
// Without this, a run that never reaches Proxy.Stop (SIGKILL, panic, SIGHUP,
// power loss) permanently leaks a live LiteLLM holding a loopback port and a
// valid master key, plus its config directory — nothing else in Phase 1
// (no worker panel, no kill switch) ever reaps it.
//
// Every failure is swallowed: an unreadable proxyRoot (including "does not
// exist yet") just means there is nothing to sweep, and this must never block
// starting the run that called it.
func sweepStaleProxyDirs(proxyRoot string) {
	entries, err := os.ReadDir(proxyRoot)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleProxyDirAge)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(proxyRoot, e.Name())
		cfgPath := filepath.Join(dir, "config.yaml")
		info, statErr := os.Stat(cfgPath)
		if statErr != nil || info.ModTime().After(cutoff) {
			// No config.yaml (not one of ours) or plausibly still live: leave
			// it alone rather than risk a live sibling.
			continue
		}
		router.ReapStale(cfgPath)
		_ = os.RemoveAll(dir)
	}
}

// ollamaAPIBase returns the api_base to verify the key against: the resolved
// api_base of the first ollama-cloud model in models, or the default when
// there is none. models is expected to already be resolved (config.
// ResolveModel), matching what the worker config itself will use — verifying
// against the hardcoded default while a user has a non-default api_base would
// pass preflight for a host the worker never actually talks to.
//
// This is only a fallback for callers that don't already know which model the
// run is using: it returns the first ollama row's api_base, which is wrong
// whenever a legacy config carries a different inline api_base per model.
// StartProxy's caller holds the resolved role model and should pass its
// api_base directly instead.
func ollamaAPIBase(models []config.Model) string {
	for _, m := range models {
		if m.Provider == config.ProviderOllama && m.APIBase != "" {
			return m.APIBase
		}
	}
	return config.OllamaDefaultAPIBase
}

// StartProxy preflights the key, writes a worker config, and starts LiteLLM.
// models must already be resolved (config.Config.ResolveModel): callers on the
// interactive path always consume resolved models, and an unresolved model can
// carry a blank APIBase/KeyEnv that the provider is meant to supply.
//
// apiBase is the resolved api_base of the specific model this run's role will
// use; VerifyKey probes it directly. When empty, it falls back to
// ollamaAPIBase(models) (see that function's doc for why that fallback can
// probe the wrong host on a legacy multi-model config).
func StartProxy(ctx context.Context, models []config.Model, masterKey, apiBase string) (*Proxy, error) {
	key, err := secrets.Get(config.OllamaKeyEnv)
	if err != nil {
		return nil, fmt.Errorf("refusing to start: could not read %s from the keychain: %w", config.OllamaKeyEnv, err)
	}
	if key == "" {
		return nil, fmt.Errorf("refusing to start: no %s in the keychain; set it in Commander's Providers panel", config.OllamaKeyEnv)
	}
	if apiBase == "" {
		apiBase = ollamaAPIBase(models)
	}
	// VerifyKey names an impossible model so 401 and 404 separate cleanly and
	// no quota is spent. It returns nil on any transport failure — inconclusive
	// by design — so only an explicit rejection is fatal here.
	if verr := ollama.VerifyKey(ctx, apiBase, key); errors.Is(verr, ollama.ErrKeyRejected) {
		return nil, fmt.Errorf("refusing to start: ollama rejected the stored %s", config.OllamaKeyEnv)
	}

	body, err := WorkerYAML(models, masterKey)
	if err != nil {
		return nil, err
	}

	proxyRoot := filepath.Join(StateDir(), "proxy")
	sweepStaleProxyDirs(proxyRoot)

	dir, err := runProxyDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create proxy dir: %w", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write worker config: %w", err)
	}
	// The callback module must sit beside the yaml: Controller.Start runs
	// litellm with cwd set to the config dir so the import resolves.
	if err := os.WriteFile(filepath.Join(dir, router.HookFile), router.HookSource(), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("write %s: %w", router.HookFile, err)
	}

	ctl := router.NewController(0)
	ctl.ConfigPath = cfgPath
	ctl.Env = []string{config.OllamaKeyEnv + "=" + key}
	if err := ctl.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	p := &Proxy{ctl: ctl, dir: dir}
	for i := 0; i < healthPollAttempts; i++ { // a cold venv start measured ~8s
		if ctl.Health() == nil {
			return p, nil
		}
		select {
		case <-ctx.Done():
			_ = p.Stop()
			return nil, ctx.Err()
		case <-time.After(healthPollInterval):
		}
	}
	_ = p.Stop()
	return nil, errors.New("litellm did not become healthy")
}

// URL is the proxy's base URL, for ANTHROPIC_BASE_URL.
func (p *Proxy) URL() string { return fmt.Sprintf("http://localhost:%d", p.ctl.Port) }

// Stop terminates the proxy and removes its per-run directory (config.yaml,
// the callback module). Idempotent.
func (p *Proxy) Stop() error {
	if p == nil || p.ctl == nil {
		return nil
	}
	err := p.ctl.Stop()
	if p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
	return err
}
