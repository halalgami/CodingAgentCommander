// Command delegate runs one scoped, read-only Ollama Cloud worker and prints a
// validated JSON result on stdout.
//
// Phase 1 is a dogfood binary: build with `go build ./cmd/delegate` and invoke
// it by absolute path. It is deliberately NOT bundled into the app — Wails
// builds only the root package main — because bundling is work this phase has
// not earned. See docs/superpowers/plans/2026-09-03-ollama-delegation.md.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/delegate"
)

type options struct {
	Role   string
	Task   string
	Paths  string
	Root   string
	Config string
}

func main() {
	var o options
	flag.StringVar(&o.Role, "role", "scout", "delegation role (scout, reviewer)")
	flag.StringVar(&o.Task, "task", "", "self-contained task brief (required)")
	flag.StringVar(&o.Paths, "paths", "", "comma-separated path hints")
	flag.StringVar(&o.Root, "root", ".", "repository root the worker may read")
	flag.StringVar(&o.Config, "config", config.UserPath(), "path to config.toml")
	flag.Parse()

	// Without this, SIGINT terminates the process immediately: the deferred
	// px.Stop() below never runs, and setPgid puts the worker in its own
	// process group, so a terminal Ctrl-C never reaches it either — both the
	// orphaned LiteLLM (still listening on loopback with a valid master key)
	// and the worker leak. NotifyContext turns the signal into ctx
	// cancellation instead, which StartProxy and Run already observe.
	//
	// SIGHUP is in this set too, and matters more than the other two in
	// practice: it's what a closed terminal or a dropped SSH session sends —
	// the ordinary way an orchestrator's shell dies — and its default action
	// terminates the process immediately, same as an unhandled SIGINT/TERM
	// would. The worker doesn't see it either, for the same process-group
	// reason noted above.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	res, err := run(ctx, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// One line per run, so phase 1's adoption question is answered with data
	// rather than someone's recollection a week later. Logged for every status:
	// a failed_infra run is still an invocation, and separating "reached for it
	// and it broke" from "never reached for it" is the whole point.
	//
	// Only reached when run() produced a Result — a refusal (empty brief,
	// unknown role, unusable catalog row) spawns no worker and is not a run.
	// A logging failure is a warning, never the run's outcome.
	mode := os.ModeNamedPipe // unknown => agent, matching InvokerFor's declared bias
	if st, statErr := os.Stdout.Stat(); statErr == nil {
		mode = st.Mode()
	}
	rec := delegate.NewRunRecord(o.Role, res, delegate.InvokerFor(mode), time.Now())
	if logErr := delegate.AppendRunTo(delegate.RunLogPath(), rec); logErr != nil {
		fmt.Fprintln(os.Stderr, "warning: could not append to the runs log:", logErr)
	}

	out, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(out))
	// A failed run is still a well-formed result on stdout; the exit code is
	// what a shell caller branches on.
	if res.Status == delegate.StatusFailedWork || res.Status == delegate.StatusFailedInfra {
		os.Exit(2)
	}
}

func run(ctx context.Context, o options) (delegate.Result, error) {
	var zero delegate.Result
	if strings.TrimSpace(o.Task) == "" {
		return zero, fmt.Errorf("refusing to run: -task is required, and must be self-contained (the worker sees no conversation history)")
	}
	role, err := delegate.RoleByName(o.Role)
	if err != nil {
		return zero, err
	}
	cfg, err := config.Load(o.Config)
	if err != nil {
		return zero, fmt.Errorf("load %s: %w", o.Config, err)
	}
	// cfg.Model alone is not enough: WorkerYAML (proxy.go) independently skips
	// any catalog row whose Provider isn't ollama-cloud, whose upstream is
	// empty, or whose upstream lacks the "ollama_chat/" prefix (config.go
	// carries a named warning that the "ollama/" provider never sends the
	// bearer token — that row would 404 at the first worker turn), so a row
	// that merely matches on ID can pass this check and still be silently
	// dropped from the generated proxy config — the proxy then starts healthy
	// and every worker turn 400s/404s on an unknown model, burning the full
	// deadline. Require exactly what the generator requires, so a bad catalog
	// row fails here instead.
	m, ok := cfg.Model(role.Model)
	if !ok || m.Provider != config.ProviderOllama ||
		!strings.HasPrefix(m.Upstream, config.OllamaUpstreamPrefix) ||
		strings.TrimPrefix(m.Upstream, config.OllamaUpstreamPrefix) == "" {
		return zero, fmt.Errorf("refusing to run role %q: its model %q is not in the catalog as a usable ollama-cloud entry at %s; ollama retires cloud models on a scale of weeks, so re-add it in Commander's Models panel", role.Name, role.Model, o.Config)
	}
	if info, statErr := os.Stat(o.Root); statErr != nil || !info.IsDir() {
		return zero, fmt.Errorf("refusing to run: -root %q is not an existing directory", o.Root)
	}
	absRoot, err := filepath.Abs(o.Root)
	if err != nil {
		return zero, err
	}

	// A fresh token per run: the proxy listens on loopback, but a predictable
	// bearer on a known port is a needless invitation.
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return zero, fmt.Errorf("generate master key: %w", err)
	}
	masterKey := "sk-delegate-" + hex.EncodeToString(buf)

	// config.ResolveModel's doc comment says launch/router/creds code paths
	// consume resolved models only: an ollama catalog row commonly has a blank
	// APIBase/KeyEnv because the provider entry supplies them (see app.go).
	// Passing cfg.Models raw meant a user with a non-default Ollama api_base
	// got that base honoured on the interactive path but silently ignored
	// here — the worker would talk to ollama.com regardless.
	resolved := make([]config.Model, len(cfg.Models))
	for i, cm := range cfg.Models {
		resolved[i] = cfg.ResolveModel(cm)
	}

	// The role's own resolved model, not delegate.ollamaAPIBase's "first
	// ollama row in the catalog" fallback: a legacy config can carry a
	// different inline api_base per model, and VerifyKey must probe the same
	// host the worker will actually talk to.
	roleModel := cfg.ResolveModel(m)
	px, err := delegate.StartProxy(ctx, resolved, masterKey, roleModel.APIBase)
	if err != nil {
		return zero, err
	}
	defer px.Stop()

	return delegate.Run(ctx, delegate.Spec{
		Role:        role,
		Task:        o.Task,
		Paths:       splitPaths(o.Paths),
		Root:        absRoot,
		ProxyURL:    px.URL(),
		MasterKey:   masterKey,
		WorkerModel: delegate.WorkerModelName(role.Model),
	})
}

func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
