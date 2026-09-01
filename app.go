package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/halalgami/CodingAgentCommander/internal/anthropic"
	"github.com/halalgami/CodingAgentCommander/internal/bedrock"
	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/deps"
	"github.com/halalgami/CodingAgentCommander/internal/hookmgr"
	"github.com/halalgami/CodingAgentCommander/internal/launch"
	"github.com/halalgami/CodingAgentCommander/internal/ollama"
	"github.com/halalgami/CodingAgentCommander/internal/pricing"
	"github.com/halalgami/CodingAgentCommander/internal/ptybridge"
	"github.com/halalgami/CodingAgentCommander/internal/router"
	"github.com/halalgami/CodingAgentCommander/internal/secrets"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
	"github.com/halalgami/CodingAgentCommander/internal/transcripts"
	"github.com/halalgami/CodingAgentCommander/internal/winstate"
	"github.com/halalgami/CodingAgentCommander/internal/wsterm"
	"github.com/halalgami/CodingAgentCommander/internal/zen"
)

// ModelInfo is the picker entry sent to the frontend.
type ModelInfo struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Routed bool   `json:"routed"`
	Ready  bool   `json:"ready"`
	// Default marks config's default_model. Without it the picker preselected
	// the first catalog entry, so an upgraded config kept preselecting whatever
	// happened to be listed first — never the new models, which merge in at the
	// end.
	Default bool `json:"default"`
}

// SessionInfo describes a launched session for the sidebar.
type SessionInfo struct {
	WindowID string `json:"windowID"`
	Name     string `json:"name"`
	Model    string `json:"model"`
}

// BuildInfo is the app version/build stamp shown in the About panel. Values are
// injected at build time via ldflags (see Makefile); defaults are dev-run values.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
}

// GetBuildInfo returns the ldflags-injected version stamp (main.appVersion etc.).
func (a *App) GetBuildInfo() BuildInfo {
	return BuildInfo{Version: appVersion, Commit: appCommit, BuildDate: appBuildDate}
}

// App is the Wails backend.
type App struct {
	ctx        context.Context
	cfg        config.Config
	host       tmux.Host
	wsPort     int
	wsListener net.Listener
	mu         sync.Mutex
	// current holds the session the single pane is showing.
	current string
	// sessions is the in-memory registry of launched sessions, keyed by
	// tmux window id. Guarded by mu.
	sessions map[string]*sessionRec
	notifier Notifier
	emitter  Emitter
	// masterKey is a random per-run secret used to authenticate to the local
	// LiteLLM proxy.
	masterKey string
	// wsToken is a random per-run secret gating the /ws terminal stream and the
	// /notify hook endpoint against other local processes / browser pages.
	wsToken string
	// configPath is where a.cfg was loaded from, and where mutations
	// (AddModel/RemoveModel) are persisted back to.
	configPath string
	// windows remembers each tmux window's model and Remote Control flag. This
	// was a pair of tmux user options until psmux turned out not to scope them
	// per window — see internal/winstate. Never nil: NewApp gives it a
	// memory-only store so a not-yet-started App is still usable.
	windows *winstate.Store
	// router is the lazily-started local LiteLLM proxy used for routed
	// (non-anthropic) models. Guarded by routerMu.
	router   *router.Controller
	routerMu sync.Mutex
	// routerHash fingerprints the resolved config (yaml+env) the running proxy
	// was started with, so ensureRouter can detect drift (a model or key added
	// after start) and restart instead of serving a stale model list.
	routerHash string
	// runtimeInstalling guards against overlapping first-run LiteLLM installs
	// (a double-clicked Install button); a second call no-ops while one runs.
	runtimeInstalling atomic.Bool
	// pwshInstalling does the same for the on-demand PowerShell 7 download,
	// which matters more there: the archive is ~106 MB, so a double click would
	// otherwise mean downloading it twice into the same staging directory.
	pwshInstalling atomic.Bool
	// statCache memoizes parsed transcript stats keyed by path, invalidated by
	// mtime, so the 5s stats poll doesn't re-parse unchanged transcripts.
	statCache map[string]statEntry
	statMu    sync.Mutex
	// historyMu guards projects.json reads/writes (project-open history).
	historyMu sync.Mutex
}

// statEntry is a cached transcript parse, valid while the file's mtime is mod.
type statEntry struct {
	mod        int64
	ctx, turns int
}

// Notifier shows a desktop notification.
type Notifier interface {
	Notify(title, body string) error
}

type osascriptNotifier struct{}

func (osascriptNotifier) Notify(title, body string) error {
	// macOS: one call does notification + sound.
	script := fmt.Sprintf(`display notification %q with title %q sound name "Ping"`, body, title)
	return exec.Command("osascript", "-e", script).Run()
}

// Emitter emits a frontend event.
type Emitter interface {
	Emit(event string, data ...any)
}

type wailsEmitter struct{ a *App }

func (w wailsEmitter) Emit(event string, data ...any) {
	if w.a.ctx != nil {
		wruntime.EventsEmit(w.a.ctx, event, data...)
	}
}

type stopPayload struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
}

// sessionRec is the registry record for one launched session.
type sessionRec struct {
	Name, Cwd, Model, Provider              string
	LaunchedAt                              time.Time
	ClaudeSessionID, TranscriptPath, Status string
	RemoteControl                           bool
	// AckMs is the ms-epoch the user last SELECTED (acknowledged) this
	// session. It is display-only and deliberately independent of Status: the
	// Stop hook fires on every assistant turn, so Status stays "finished" for
	// the rest of the process's life, and SelectSession must never rewrite it
	// (see its doc comment).
	AckMs int64
}

// SessionStats is the per-session card data.
type SessionStats struct {
	ContextTokens  int     `json:"contextTokens"`
	EstCostPerTurn float64 `json:"estCostPerTurn"`
	// Unpriced means the catalog carries no rate, so the card must show no
	// dollar figure. An explicit flag rather than an EstCostPerTurn == 0 check
	// in the frontend, which would also blank a genuinely free first turn on a
	// priced model.
	Unpriced      bool   `json:"unpriced"`
	Band          string `json:"band"`
	Turns         int    `json:"turns"`
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	UptimeSeconds int    `json:"uptimeSeconds"`
	Status        string `json:"status"`
	RemoteControl bool   `json:"remoteControl"`
	Cwd           string `json:"cwd"`
}

// now is a tiny seam so tests don't depend on wall clock.
func (a *App) now() time.Time { return time.Now() }

// projectsRoot is ~/.claude/projects (overridable in tests).
func (a *App) projectsRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// NewApp constructs the backend.
func NewApp() *App {
	a := &App{host: tmux.NewExecHost(), sessions: map[string]*sessionRec{}, notifier: nativeNotifier{},
		windows: winstate.Open("")}
	a.emitter = wailsEmitter{a: a}
	return a
}

// handleNotify processes a Stop-hook payload: update the matching session and,
// if it isn't the focused pane, desktop-notify + emit the finished event.
func (a *App) handleNotify(body []byte) {
	var p stopPayload
	if json.Unmarshal(body, &p) != nil {
		return
	}
	a.mu.Lock()
	var id string
	var rec *sessionRec
	for wid, r := range a.sessions {
		if r.Cwd == p.Cwd {
			id, rec = wid, r
			break
		}
	}
	if rec == nil {
		a.mu.Unlock()
		return // not a Commander session
	}
	rec.ClaudeSessionID = p.SessionID
	if p.TranscriptPath != "" {
		rec.TranscriptPath = p.TranscriptPath
	}
	rec.Status = "finished"
	focused := a.current == id
	name := rec.Name
	a.mu.Unlock()

	a.emitter.Emit("session:finished", id)
	if !focused {
		_ = a.notifier.Notify("Commander", "Claude finished in "+name)
	}
}

// settingsPath is ~/.claude/settings.json, where Claude Code hooks live.
func (a *App) settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// shutdown is the Wails OnShutdown hook: remove Commander's Stop hook, stop the
// router, and clean up the generated router files so nothing lingers on disk.
func (a *App) shutdown(ctx context.Context) {
	_ = hookmgr.Remove(a.settingsPath())
	if a.wsListener != nil {
		_ = a.wsListener.Close() // stop the local http server (was leaked before)
	}
	if a.router != nil {
		_ = a.router.Stop()
	}
	// Belt-and-suspenders: reap any litellm worker Stop() may have missed, then
	// remove the generated yaml + hook (regenerated fresh on next launch).
	router.ReapStale(a.litellmConfigPath())
	a.removeRouterFiles()
}

// removeRouterFiles deletes the generated LiteLLM config and its callback hook.
func (a *App) removeRouterFiles() {
	p := a.litellmConfigPath()
	_ = os.Remove(p)
	_ = os.Remove(filepath.Join(filepath.Dir(p), router.HookFile))
}

func configPath() string {
	if p := os.Getenv("COMMANDER_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "commander", "config.toml")
}

// loadConfigFrom loads the catalog. A missing file is first-run, not an
// error: Commander seeds the native-Anthropic starter catalog and writes it,
// so a fresh install boots launchable with zero setup (subscription users
// need no keys). Any other load error (bad TOML, empty catalog) still fails.
func (a *App) loadConfigFrom(path string) error {
	c, err := config.Load(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		c = config.Default()
		if serr := config.Save(path, c); serr != nil {
			return serr
		}
	}
	a.cfg = c
	a.configPath = path
	return nil
}

// reportError surfaces a non-fatal failure to the user as a toast.
//
// Commander is a GUI binary with no console attached, so log output goes
// nowhere anyone will read: a catalog merge that failed to persist looked
// exactly like one that worked, and the picker just stayed stale. Emitting
// means the one person who can act on it finds out.
//
// Safe before startup and in tests, where there is no context to emit on:
// wailsEmitter.Emit is already nil-ctx guarded (app.go:169-173), and routing
// through the seam makes the failure paths assertable.
func (a *App) reportError(msg string) {
	if a.emitter == nil {
		return
	}
	a.emitter.Emit("app:error", msg)
}

// startup is the Wails OnStartup hook: load config and start the ws server.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Finder/Dock launches inherit launchd's minimal PATH; make homebrew,
	// pip-user, and /etc/paths.d locations visible so tmux/claude/litellm
	// resolve the same as from a terminal.
	launch.AugmentPATH()
	if err := a.loadConfigFrom(configPath()); err != nil {
		// Surface via an empty picker; the UI shows a config hint.
		return
	}
	a.refreshAnthropicModels()
	// Background: needs the network, and nothing downstream waits on it. Kept
	// out of refreshAnthropicModels so that stays synchronous and testable.
	go a.discoverAnthropicModels()
	a.masterKey = randomHex(24)
	a.wsToken = randomHex(24)
	// Reap any litellm orphaned by a prior unclean exit before starting fresh.
	router.ReapStale(a.litellmConfigPath())
	a.router = router.NewController(0)
	a.startWS()
	_ = hookmgr.Install(a.settingsPath(), a.wsPort, a.wsToken)
	a.windows = winstate.Open(a.winStatePath())
	a.reconcile(a.reconcileWindows()) // adopt any windows surviving from a prior run
}

// refreshAnthropicModels keeps the native side of the catalog current.
//
// Two passes, for two different kinds of staleness. The built-in catalog merges
// synchronously — no network — so the picker is right before the frontend's
// first Config() call, and an install carrying a config written by an older
// build picks up this release's models without hand-editing TOML. Then, if an
// ANTHROPIC_API_KEY is in the environment, startup runs discovery in the
// background for models released since the build, emitting models:updated so the
// picker reloads.
//
// Add-only in both passes: a model the user renamed or repriced keeps their
// edits, and only ids absent from the catalog are appended. Deletions survive
// too — the built-in pass is gated on anthropic.CatalogRev, so a model removed
// on purpose only comes back after an upgrade that changed the list. Live
// discovery has no such gate: it cannot tell "released since the build" from
// "deleted by the user", and re-adding is the less surprising of the two.
func (a *App) refreshAnthropicModels() {
	a.mu.Lock()
	stale := a.cfg.AnthropicCatalogRev < anthropic.CatalogRev
	a.mu.Unlock()
	if stale {
		if err := a.mergeAnthropic(config.AnthropicModels(), anthropic.CatalogRev); err != nil {
			// Not worth failing launch over — the picker still shows whatever
			// the config already had — but the user has to be told, or a stale
			// picker is indistinguishable from a current one.
			a.reportError(fmt.Sprintf("model catalog could not be updated: %v", err))
		}
	}
}

// discoverAnthropicModels merges any models an ANTHROPIC_API_KEY in the
// environment can see that the build does not know about.
//
// The environment is the only source: Anthropic is not a config Provider — it is
// the built-in, subscription-authenticated one — so there is no Providers row to
// paste a key into and nothing ever writes this ref to the keychain. Reading it
// from there anyway would be dead code dressed as a feature. A persistent user
// environment variable is inherited by Explorer launches on Windows, and a
// terminal launch works anywhere; installs with no key at all just keep the
// built-in catalog, which is the common case and why that catalog carries the
// weight.
func (a *App) discoverAnthropicModels() {
	key := os.Getenv(anthropic.KeyEnv)
	if key == "" {
		return
	}
	found, err := anthropic.ListModels(a.ctx, key)
	if err != nil {
		a.reportError(fmt.Sprintf("could not check Anthropic for new models: %v", err))
		return
	}
	models := make([]config.Model, 0, len(found))
	for _, m := range found {
		models = append(models, config.Model{
			ID: m.ID, Label: m.Label, Provider: config.ProviderAnthropic,
			InputPrice: m.InputPrice, OutputPrice: m.OutputPrice,
		})
	}
	// Revision unchanged: discovery is not the built-in catalog, and stamping it
	// here would suppress the next build's merge.
	if err := a.mergeAnthropic(models, 0); err != nil {
		a.reportError(fmt.Sprintf("discovered models could not be saved: %v", err))
		return
	}
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "models:updated")
	}
}

// mergeAnthropic appends the models whose ids are not already in the catalog and
// persists before committing, matching every other catalog mutation. A rev of 0
// leaves AnthropicCatalogRev alone. Returns nil when there is nothing to do.
func (a *App) mergeAnthropic(models []config.Model, rev int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	have := make(map[string]bool, len(a.cfg.Models))
	for _, m := range a.cfg.Models {
		have[m.ID] = true
	}
	next := a.cfg
	next.Models = append([]config.Model{}, a.cfg.Models...)
	added := 0
	for _, m := range models {
		if have[m.ID] {
			continue
		}
		have[m.ID] = true
		next.Models = append(next.Models, m)
		added++
	}
	if rev > next.AnthropicCatalogRev {
		next.AnthropicCatalogRev = rev
	} else if added == 0 {
		return nil
	}
	if a.configPath == "" {
		return nil // no config on disk yet (tests)
	}
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next // commit only after successful persist
	return nil
}

// reconcileWindows lists the live tmux windows (helper so startup reconciles
// before the frontend's first ListSessions call).
func (a *App) reconcileWindows() []tmux.WindowState {
	ws, _ := a.host.List(a.cfg.TmuxSession)
	return ws
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// routerConfigAndEnv builds the LiteLLM config for the catalog's routed models
// and resolves each model's key_env from the keychain. Returns the yaml, the
// KEY_ENV=value env entries for the proxy process, and the list of key_envs
// that have no stored key (missing).
func (a *App) routerConfigAndEnv() ([]byte, []string, []string, error) {
	models := a.snapshotModels()
	seen := map[string]bool{}
	keyOK := map[string]bool{}
	var env, missing []string
	// inject resolves ref from the keychain (once) into the proxy env; returns
	// whether the key is present. Safe to call repeatedly for the same ref.
	inject := func(ref string) bool {
		if !seen[ref] {
			seen[ref] = true
			if val, gerr := secrets.Get(ref); gerr == nil && val != "" {
				keyOK[ref] = true
				env = append(env, ref+"="+val)
			}
		}
		return keyOK[ref]
	}
	for _, m := range models {
		if !m.IsRouted() {
			continue
		}
		for _, ref := range m.CredEnvs() { // required
			if !inject(ref) && !contains(missing, ref) {
				missing = append(missing, ref)
			}
		}
		for _, ref := range m.OptionalCredEnvs() { // best-effort, never "missing"
			inject(ref)
		}
	}
	var ready []config.Model
	for _, m := range models {
		if m.IsRouted() && credsPresent(m, keyOK) {
			ready = append(ready, m)
		}
	}
	opts := router.Options{AWSSessionToken: keyOK[config.AWSSessionTokenEnv]}
	if skip := router.ThinkingSkipIDs(ready); len(skip) > 0 {
		env = append(env, router.SkipThinkingEnv+"="+strings.Join(skip, ","))
	}
	yaml, err := router.GenerateConfig(ready, a.masterKey, opts)
	if err != nil {
		return nil, nil, nil, err
	}
	return yaml, env, missing, nil
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// credsPresent reports whether every keychain ref the model needs is in have.
func credsPresent(m config.Model, have map[string]bool) bool {
	for _, ref := range m.CredEnvs() {
		if !have[ref] {
			return false
		}
	}
	return true
}

// modelReady reports whether every credential the model needs is in the
// keychain; on the first missing one it returns that ref for the error message.
func modelReady(m config.Model) (bool, string) {
	for _, ref := range m.CredEnvs() {
		v, err := secrets.Get(ref)
		if err != nil || v == "" {
			return false, ref
		}
	}
	return true, ""
}

// LitellmRuntimeStatus reports whether the LiteLLM proxy runtime is resolvable
// (system pip install or the app-managed venv) and, if not, whether a first-run
// install can build it here. The frontend calls this to decide whether to prompt.
func (a *App) LitellmRuntimeStatus() router.RuntimeStatus { return router.Status() }

// InstallLitellmRuntime builds the managed LiteLLM runtime in the background,
// streaming progress to the frontend via events:
//
//	litellm-install:log    string  — one line of venv/pip output
//	litellm-install:done   (none)  — install succeeded
//	litellm-install:error  string  — install failed, with the message
//
// It returns immediately; the frontend listens for the terminal event. A second
// call while an install is in flight is a no-op.
func (a *App) InstallLitellmRuntime() {
	if !a.runtimeInstalling.CompareAndSwap(false, true) {
		return // an install is already running
	}
	go func() {
		defer a.runtimeInstalling.Store(false)
		emit := func(line string) { wruntime.EventsEmit(a.ctx, "litellm-install:log", line) }
		if err := router.InstallRuntime(a.ctx, "", emit); err != nil {
			wruntime.EventsEmit(a.ctx, "litellm-install:error", err.Error())
			return
		}
		wruntime.EventsEmit(a.ctx, "litellm-install:done")
	}()
}

// DependencyStatus reports every external tool Commander needs — tmux (psmux on
// Windows), pwsh 7 there too, and the claude CLI — with the resolved path and an
// install hint for the ones that are missing.
//
// The frontend calls this on startup so a missing dependency surfaces as a named
// prompt rather than as `exec: "tmux": executable file not found in %PATH%` on
// the first launch attempt.
func (a *App) DependencyStatus() []deps.Tool { return deps.Status() }

// InstallPwsh downloads the pinned PowerShell 7 into the managed runtime dir in
// the background, streaming progress to the frontend via events:
//
//	pwsh-install:log    string  — one line of download/extract progress
//	pwsh-install:done   (none)  — install succeeded
//	pwsh-install:error  string  — install failed, with the message
//
// It returns immediately; the frontend listens for the terminal event. A second
// call while an install is in flight is a no-op. Windows-only in practice — off
// Windows deps.InstallPwsh reports that pwsh is not required.
func (a *App) InstallPwsh() {
	if !a.pwshInstalling.CompareAndSwap(false, true) {
		return // an install is already running
	}
	go func() {
		defer a.pwshInstalling.Store(false)
		emit := func(line string) { wruntime.EventsEmit(a.ctx, "pwsh-install:log", line) }
		if err := deps.InstallPwsh(a.ctx, emit); err != nil {
			wruntime.EventsEmit(a.ctx, "pwsh-install:error", err.Error())
			return
		}
		wruntime.EventsEmit(a.ctx, "pwsh-install:done")
	}()
}

// winStatePath is where per-window state (model, Remote Control) is persisted.
func (a *App) winStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "commander", "windows.json")
}

// litellmConfigPath is where the generated LiteLLM config.yaml is written.
func (a *App) litellmConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "commander", "litellm.yaml")
}

// ensureRouter starts the LiteLLM proxy, injecting provider keys, and errors
// (without starting anything) if a required key is missing. If the proxy is
// already running but the resolved config has since drifted — a routed model or
// its key was added after start — it restarts on the same port so the new model
// list takes effect (otherwise that model would 404).
func (a *App) ensureRouter() error {
	a.routerMu.Lock()
	defer a.routerMu.Unlock()
	yaml, env, _, err := a.routerConfigAndEnv()
	if err != nil {
		return err
	}
	h := hashConfig(yaml, env)
	if a.router.Running() {
		if h == a.routerHash {
			return nil // already serving exactly this config
		}
		_ = a.router.Stop() // config drifted; restart with the new model list
	}
	p := a.litellmConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, yaml, 0o600); err != nil {
		return err
	}
	// The generated config references the strip_thinking callback; write the
	// module next to the yaml so litellm (run from this dir) can import it.
	hookPath := filepath.Join(filepath.Dir(p), router.HookFile)
	if err := os.WriteFile(hookPath, router.HookSource(), 0o644); err != nil {
		return err
	}
	a.router.ConfigPath = p
	a.router.Env = env
	if err := a.router.Start(); err != nil {
		return err
	}
	for i := 0; i < 100; i++ {
		if a.router.Health() == nil {
			a.routerHash = h
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = a.router.Stop() // reset so a later launch can retry
	a.routerHash = ""
	return fmt.Errorf("LiteLLM did not become healthy")
}

// hashConfig fingerprints the generated yaml plus the (order-independent) env so
// ensureRouter can tell whether the running proxy is serving the current config.
func hashConfig(yaml []byte, env []string) string {
	sorted := append([]string(nil), env...)
	sort.Strings(sorted)
	h := sha256.New()
	h.Write(yaml)
	for _, e := range sorted {
		h.Write([]byte{0})
		h.Write([]byte(e))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// mediaHandler is the asset server's fallback handler: wails calls it for any
// GET the embedded assets answer with os.ErrNotExist. This build registers no
// media of its own, so every such request is a 404.
func (a *App) mediaHandler() http.Handler { return http.NotFoundHandler() }

func (a *App) startWS() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return
	}
	a.wsPort = ln.Addr().(*net.TCPAddr).Port
	a.wsListener = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsterm.Handler(a.wsToken, func() (*ptybridge.Bridge, error) {
		a.mu.Lock()
		cur := a.current
		a.mu.Unlock()
		if cur == "" {
			return nil, fmt.Errorf("no session selected")
		}
		b, err := ptybridge.Attach(a.cfg.TmuxSession, 50, 200)
		if err != nil {
			return nil, err
		}
		if cur != "" {
			_ = b.SelectWindow(cur)
		}
		return b, nil
	}))
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		if a.wsToken != "" && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(a.wsToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		a.handleNotify(body)
		w.WriteHeader(http.StatusOK)
	})
	go func() { _ = http.Serve(ln, mux) }()
}

// WSPort returns the local websocket port for the terminal stream.
func (a *App) WSPort() int { return a.wsPort }

// WSToken returns the per-run token the frontend must present on the /ws stream.
func (a *App) WSToken() string { return a.wsToken }

// snapshotModels returns a copy of the catalog under lock, each model resolved
// against its provider (safe to range lock-free).
func (a *App) snapshotModels() []config.Model {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]config.Model, 0, len(a.cfg.Models))
	for _, m := range a.cfg.Models {
		out = append(out, a.cfg.ResolveModel(m))
	}
	return out
}

// modelByID looks up a model under lock, resolved against its provider.
func (a *App) modelByID(id string) (config.Model, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, ok := a.cfg.Model(id)
	if !ok {
		return config.Model{}, false
	}
	return a.cfg.ResolveModel(m), true
}

// Config returns all models for the picker (native + routed), each flagged
// Routed and Ready (key present for routed; native always ready).
func (a *App) Config() []ModelInfo {
	a.mu.Lock()
	def := a.cfg.DefaultModel
	a.mu.Unlock()
	out := []ModelInfo{}
	for _, m := range a.snapshotModels() {
		ready, _ := modelReady(m)
		out = append(out, ModelInfo{
			ID: m.ID, Label: m.Label, Routed: m.IsRouted(), Ready: ready,
			Default: m.ID == def,
		})
	}
	return out
}

// KeyInfo is a provider key slot for the admin panel.
type KeyInfo struct {
	Env      string `json:"env"`
	Set      bool   `json:"set"`
	Optional bool   `json:"optional"`
}

// ProviderInfo describes one definable provider type for the admin panel.
type ProviderInfo struct {
	Type     string `json:"type"`     // "opencode-go" | "bedrock" | "ollama-cloud"
	Defined  bool   `json:"defined"`  // [[providers]] entry exists
	Active   bool   `json:"active"`   // defined && required keys set
	APIBase  string `json:"apiBase"`  // zen or Ollama (default prefilled when undefined)
	Region   string `json:"region"`   // bedrock
	ModelCnt int    `json:"modelCnt"` // catalog models of this type (for remove confirm)
}

// KeyStatus lists credential slots for the DEFINED providers. Slots appear the
// moment a provider is defined — before any of its models exist — which is the
// whole point of the provider-first flow.
func (a *App) KeyStatus() []KeyInfo {
	a.mu.Lock()
	providers := append([]config.Provider{}, a.cfg.Providers...)
	a.mu.Unlock()
	out := []KeyInfo{}
	add := func(ref string, optional bool) {
		v, err := secrets.Get(ref)
		out = append(out, KeyInfo{Env: ref, Set: err == nil && v != "", Optional: optional})
	}
	for _, p := range providers {
		switch p.Type {
		case config.ProviderOpencodeGo:
			add(config.ZenKeyEnv, false)
		case config.ProviderBedrock:
			add(config.AWSAccessKeyEnv, false)
			add(config.AWSSecretKeyEnv, false)
			add(config.AWSSessionTokenEnv, true)
		case config.ProviderOllama:
			add(config.OllamaKeyEnv, false)
		}
	}
	return out
}

// DiscoverBedrockModels lists the text models the stored AWS credentials can
// invoke in region (default us-east-1), for one-click add to the catalog. Reads
// the AWS keys from the keychain — set them in Providers first.
func (a *App) DiscoverBedrockModels(region string) ([]bedrock.Model, error) {
	ak, _ := secrets.Get(config.AWSAccessKeyEnv)
	sk, _ := secrets.Get(config.AWSSecretKeyEnv)
	token, _ := secrets.Get(config.AWSSessionTokenEnv) // optional
	return bedrock.ListModels(a.ctx, ak, sk, token, region)
}

// DiscoverZenModels lists the models the stored ZEN_KEY can use on the defined
// OpenCode Zen provider.
func (a *App) DiscoverZenModels() ([]zen.Model, error) {
	a.mu.Lock()
	p, ok := a.cfg.ProviderByType(config.ProviderOpencodeGo)
	a.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("define the OpenCode Zen provider first")
	}
	key, _ := secrets.Get(config.ZenKeyEnv)
	return zen.ListModels(a.ctx, p.APIBase, key)
}

// OllamaModel is one discovered Ollama Cloud model. It carries the catalog ID
// and upstream already computed, so the drawer adds them verbatim rather than
// deriving either — its own derivation mangles dots and colons.
type OllamaModel struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Upstream string `json:"upstream"`
}

// DiscoverOllamaModels lists the models Ollama Cloud currently serves, and
// checks the stored key while it is there.
//
// The listing itself is anonymous, so it would succeed with no key at all. That
// is why the key check is bolted on here rather than trusted from the listing:
// discovery is the moment the other providers surface a bad key (Zen 401s,
// Bedrock fails signing), and without it Ollama's first sign of a wrong or
// revoked key would be a 401 from the proxy mid-turn, in a session the user has
// already opened.
func (a *App) DiscoverOllamaModels() ([]OllamaModel, error) {
	a.mu.Lock()
	p, ok := a.cfg.ProviderByType(config.ProviderOllama)
	a.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("define the Ollama Cloud provider first")
	}
	base := p.APIBase
	if base == "" {
		base = config.OllamaDefaultAPIBase
	}
	found, err := ollama.ListModels(a.ctx, base)
	if err != nil {
		return nil, err
	}
	// Read the key outside a.mu: secrets.Get hits the keychain and can block on
	// a macOS unlock prompt, and a.mu gates the whole app.
	key, _ := secrets.Get(config.OllamaKeyEnv)
	if err := ollama.VerifyKey(a.ctx, base, key); err != nil {
		return nil, fmt.Errorf("%s was rejected by %s — the model list is public, but launching needs a valid key", config.OllamaKeyEnv, base)
	}
	out := make([]OllamaModel, 0, len(found))
	for _, m := range found {
		up := config.NormalizeOllamaUpstream(m.ID)
		out = append(out, OllamaModel{ID: config.OllamaCatalogID(up), Label: m.Label, Upstream: up})
	}
	return out, nil
}

// SetKey stores a provider API key in the keychain.
func (a *App) SetKey(env, value string) error { return secrets.Set(env, value) }

// ClearKey removes a provider API key from the keychain.
func (a *App) ClearKey(env string) error { return secrets.Delete(env) }

// ModelInput is the add-model form payload.
type ModelInput struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Provider    string  `json:"provider"`
	Upstream    string  `json:"upstream"`
	APIBase     string  `json:"apiBase"`
	KeyEnv      string  `json:"keyEnv"`
	Region      string  `json:"region"`
	InputPrice  float64 `json:"inputPrice"`
	OutputPrice float64 `json:"outputPrice"`
}

// ModelDetail is a full catalog row for the admin panel.
type ModelDetail struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Provider    string  `json:"provider"`
	Routed      bool    `json:"routed"`
	Upstream    string  `json:"upstream"`
	APIBase     string  `json:"apiBase"`
	KeyEnv      string  `json:"keyEnv"`
	Region      string  `json:"region"`
	InputPrice  float64 `json:"inputPrice"`
	OutputPrice float64 `json:"outputPrice"`
}

// Models returns the full catalog for the admin panel.
func (a *App) Models() []ModelDetail {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []ModelDetail{}
	for _, m := range a.cfg.Models {
		out = append(out, ModelDetail{
			ID: m.ID, Label: m.Label, Provider: m.Provider, Routed: m.IsRouted(),
			Upstream: m.Upstream, APIBase: m.APIBase, KeyEnv: m.KeyEnv, Region: m.Region,
			InputPrice: m.InputPrice, OutputPrice: m.OutputPrice,
		})
	}
	return out
}

// ListProviders reports the definable provider types for the admin panel:
// whether each is defined, active (defined + required keys set), its stored
// endpoint/region, and how many catalog models use it (for remove confirm).
func (a *App) ListProviders() []ProviderInfo {
	a.mu.Lock()
	byType := map[string]config.Provider{}
	for _, p := range a.cfg.Providers {
		byType[p.Type] = p
	}
	modelCnt := map[string]int{}
	for _, m := range a.cfg.Models {
		modelCnt[m.Provider]++
	}
	a.mu.Unlock()

	// secrets.Get hits the keychain and can block on a macOS unlock prompt;
	// do it lock-free so it can't freeze the whole app (a.mu gates everything).
	keySet := func(ref string) bool { v, err := secrets.Get(ref); return err == nil && v != "" }
	out := []ProviderInfo{}
	for _, t := range config.DefinableProviderTypes {
		p, defined := byType[t]
		info := ProviderInfo{Type: t, Defined: defined, APIBase: p.APIBase, Region: p.Region, ModelCnt: modelCnt[t]}
		if !defined {
			switch t {
			case config.ProviderOpencodeGo:
				info.APIBase = config.ZenDefaultAPIBase
			case config.ProviderOllama:
				info.APIBase = config.OllamaDefaultAPIBase
			}
		}
		switch t {
		case config.ProviderOpencodeGo:
			info.Active = defined && keySet(config.ZenKeyEnv)
		case config.ProviderBedrock:
			info.Active = defined && keySet(config.AWSAccessKeyEnv) && keySet(config.AWSSecretKeyEnv)
		case config.ProviderOllama:
			// Key PRESENCE only: Ollama's listing endpoint is anonymous, so
			// nothing here has ever exercised the key. The drawer says "key
			// stored" rather than "active" for this provider so the difference
			// is not hidden.
			info.Active = defined && keySet(config.OllamaKeyEnv)
		}
		out = append(out, info)
	}
	return out
}

// AddProvider defines a new provider entry, persisting before it is committed
// to the in-memory catalog.
func (a *App) AddProvider(ptype, apiBase, region string) error {
	ptype = strings.TrimSpace(ptype)
	if !slices.Contains(config.DefinableProviderTypes, ptype) {
		return fmt.Errorf("unknown provider type %q", ptype)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.cfg.ProviderByType(ptype); ok {
		return fmt.Errorf("provider %q already defined", ptype)
	}
	p := config.Provider{Type: ptype}
	switch ptype {
	case config.ProviderOpencodeGo:
		p.APIBase = strings.TrimSpace(apiBase)
		if p.APIBase == "" {
			p.APIBase = config.ZenDefaultAPIBase
		}
	case config.ProviderBedrock:
		p.Region = strings.TrimSpace(region)
		if p.Region == "" {
			p.Region = "us-east-1"
		}
	case config.ProviderOllama:
		p.APIBase = strings.TrimSpace(apiBase)
		if p.APIBase == "" {
			p.APIBase = config.OllamaDefaultAPIBase
		}
	}
	next := a.cfg
	next.Providers = append(append([]config.Provider{}, a.cfg.Providers...), p)
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next
	return nil
}

// RemoveProvider drops a provider entry and its models, persisting before it
// is committed to the in-memory catalog. Refuses if one of its models is the
// default model.
func (a *App) RemoveProvider(ptype string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.cfg.ProviderByType(ptype); !ok {
		return fmt.Errorf("provider %q not defined", ptype)
	}
	for _, m := range a.cfg.Models {
		if m.Provider == ptype && m.ID == a.cfg.DefaultModel {
			return fmt.Errorf("cannot remove %s: its model %q is the default model", ptype, m.ID)
		}
	}
	next := a.cfg
	next.Providers = nil
	for _, p := range a.cfg.Providers {
		if p.Type != ptype {
			next.Providers = append(next.Providers, p)
		}
	}
	next.Models = nil
	for _, m := range a.cfg.Models {
		if m.Provider != ptype {
			next.Models = append(next.Models, m)
		}
	}
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next
	return nil
}

// providerDefined reports whether ptype has a [[providers]] entry. Caller must
// hold a.mu.
func (a *App) providerDefined(ptype string) (config.Provider, bool) {
	return a.cfg.ProviderByType(ptype)
}

// AddModel validates and appends a catalog model, persisting before it is
// committed to the in-memory catalog.
func (a *App) AddModel(in ModelInput) error {
	in.ID = strings.TrimSpace(in.ID)
	in.Provider = strings.TrimSpace(in.Provider)
	if in.ID == "" || in.Provider == "" {
		return fmt.Errorf("id and provider are required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch in.Provider {
	case config.ProviderAnthropic:
		in.Upstream, in.APIBase, in.KeyEnv, in.Region = "", "", "", "" // native carries no routed fields
	case config.ProviderBedrock:
		// Credentials come from the shared AWS_* keychain refs (set in Providers),
		// so no key_env/api_base — only the bedrock/ model string; region is
		// per-model (from discovery/form) and defaults from the provider entry
		// at resolution time if left empty.
		if _, ok := a.providerDefined(config.ProviderBedrock); !ok {
			return fmt.Errorf("define the Bedrock provider in Providers first")
		}
		in.Upstream = strings.TrimSpace(in.Upstream)
		if in.Upstream == "" {
			return fmt.Errorf("bedrock models require upstream (bedrock/<model-id>)")
		}
		in.Upstream = config.NormalizeBedrockUpstream(in.Upstream)
		in.APIBase, in.KeyEnv = "", "" // region kept: per-model, from discovery or form
	case config.ProviderOpencodeGo:
		if _, ok := a.providerDefined(config.ProviderOpencodeGo); !ok {
			return fmt.Errorf("define the OpenCode Zen provider in Providers first")
		}
		if in.Upstream == "" {
			return fmt.Errorf("routed models require upstream")
		}
		in.APIBase, in.KeyEnv, in.Region = "", "", "" // provider supplies these
	case config.ProviderOllama:
		if _, ok := a.providerDefined(config.ProviderOllama); !ok {
			return fmt.Errorf("define the Ollama Cloud provider in Providers first")
		}
		in.Upstream = strings.TrimSpace(in.Upstream)
		if in.Upstream == "" {
			return fmt.Errorf("ollama models require upstream (the model name, e.g. glm-5.3)")
		}
		in.Upstream = config.NormalizeOllamaUpstream(in.Upstream)
		// The ID is DERIVED, never taken from the form: the drawer's manual-add
		// path mangles dots and colons, so the same model added two ways would
		// otherwise land as two catalog rows pointing at one upstream.
		in.ID = config.OllamaCatalogID(in.Upstream)
		in.APIBase, in.KeyEnv, in.Region = "", "", "" // provider supplies these
	default:
		return fmt.Errorf("unknown provider %q", in.Provider)
	}
	if _, ok := a.cfg.Model(in.ID); ok {
		return fmt.Errorf("model %q already exists", in.ID)
	}
	next := a.cfg
	next.Models = append(append([]config.Model{}, a.cfg.Models...), config.Model{
		ID: in.ID, Label: in.Label, Provider: in.Provider,
		Upstream: in.Upstream, APIBase: in.APIBase, KeyEnv: in.KeyEnv, Region: in.Region,
		InputPrice: in.InputPrice, OutputPrice: in.OutputPrice,
	})
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next // commit only after successful persist
	return nil
}

// RemoveModel drops a model (never the default), persisting before it is
// committed to the in-memory catalog.
func (a *App) RemoveModel(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == a.cfg.DefaultModel {
		return fmt.Errorf("cannot remove the default model")
	}
	kept := make([]config.Model, 0, len(a.cfg.Models))
	found := false
	for _, m := range a.cfg.Models {
		if m.ID == id {
			found = true
			continue
		}
		kept = append(kept, m)
	}
	if !found {
		return fmt.Errorf("model %q not found", id)
	}
	next := a.cfg
	next.Models = kept
	if err := config.Save(a.configPath, next); err != nil {
		return err
	}
	a.cfg = next
	return nil
}

// startSession creates a session window for model m in folder, running claude
// with extraArgs (nil = fresh; []string{"--resume", id} = resume), records it in
// the registry, and sets a.current. Native vs routed env handled here.
func (a *App) startSession(folder string, m config.Model, extraArgs []string) (SessionInfo, error) {
	var env map[string]string
	var err error
	if m.IsRouted() {
		if ok, ref := modelReady(m); !ok {
			return SessionInfo{}, fmt.Errorf("model %q needs key: set %s in Providers", m.ID, ref)
		}
		if err = a.ensureRouter(); err != nil {
			return SessionInfo{}, err
		}
		env, err = launch.RoutedEnv(m, a.router.Port, a.masterKey)
	} else {
		env, err = launch.Env(m)
	}
	if err != nil {
		return SessionInfo{}, err
	}
	// Window name = project folder, so cards are tellable apart; the model
	// already shows on the card's badge (and via @commander_model for swap).
	name := filepath.Base(folder)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = m.Label
		if name == "" {
			name = m.ID
		}
	}
	cmd := append(append([]string{}, launch.Command()...), extraArgs...)
	w, err := a.host.Launch(tmux.LaunchSpec{
		SessionName: a.cfg.TmuxSession, WindowName: name, Dir: folder, Env: env, Command: cmd,
		// Every variable Commander can set, not just this launch's: a native
		// launch sets only ANTHROPIC_MODEL, and would otherwise inherit the
		// proxy vars a routed launch left on the session.
		ClearEnv: launch.EnvKeys(),
	})
	if err != nil {
		return SessionInfo{}, err
	}
	// A launch is authoritative for this window id, overwriting whatever an
	// earlier window with the same id left behind.
	if serr := a.windows.Set(w.ID, winstate.Record{Model: m.ID}); serr != nil {
		a.reportError(fmt.Sprintf("could not record the model for %s: %v", name, serr))
	}
	a.mu.Lock()
	a.sessions[w.ID] = &sessionRec{
		Name: name, Cwd: folder, Model: m.ID, Provider: m.Provider, LaunchedAt: a.now(), Status: "active",
	}
	a.current = w.ID
	a.mu.Unlock()
	return SessionInfo{WindowID: w.ID, Name: w.Name, Model: m.ID}, nil
}

// LaunchSession launches a session; remoteControl additionally enables Remote
// Control at claude startup (native Anthropic sessions only).
func (a *App) LaunchSession(folder, modelID string, remoteControl bool) (SessionInfo, error) {
	m, ok := a.modelByID(modelID)
	if !ok {
		err := fmt.Errorf("unknown model %q", modelID)
		a.reportError(err.Error())
		return SessionInfo{}, err
	}
	var extra []string
	if remoteControl {
		if m.IsRouted() {
			err := fmt.Errorf("remote control needs a native Anthropic session")
			a.reportError(err.Error())
			return SessionInfo{}, err
		}
		name := m.Label
		if name == "" {
			name = m.ID
		}
		extra = []string{"--remote-control", name}
	}
	s, err := a.startSession(folder, m, extra)
	if err != nil {
		a.reportError(fmt.Sprintf("could not launch %s: %v", filepath.Base(folder), err))
		return s, err
	}
	if remoteControl {
		a.mu.Lock()
		if rec := a.sessions[s.WindowID]; rec != nil {
			rec.RemoteControl = true
		}
		a.mu.Unlock()
		_ = a.windows.Update(s.WindowID, func(r *winstate.Record) { r.RemoteControl = true })
	}
	// err is guaranteed nil here: the only earlier assignment to it returns
	// on failure above, so an `if err == nil` guard here would always be true.
	a.recordProjectOpen(folder, modelID) // remember this project for the history
	return s, nil
}

// EnableRemoteControl types /remote-control into a native session so it can be
// continued from the Claude mobile app / claude.ai/code. If claude is mid-turn
// the command queues at the input line until the prompt returns — harmless.
// Routed sessions are rejected: Remote Control hard-fails when
// ANTHROPIC_BASE_URL is overridden to a proxy (claude-code v2.1.196+).
func (a *App) EnableRemoteControl(windowID string) error {
	a.mu.Lock()
	rec, ok := a.sessions[windowID]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("unknown session %q", windowID)
	}
	provider, name := rec.Provider, rec.Name
	a.mu.Unlock()
	if provider != config.ProviderAnthropic {
		return fmt.Errorf("remote control needs a native Anthropic session (this one routes via %s)", provider)
	}
	if err := a.host.SendKeys(windowID, "/remote-control "+name); err != nil {
		return err
	}
	_ = a.windows.Update(windowID, func(r *winstate.Record) { r.RemoteControl = true })
	a.mu.Lock()
	rec.RemoteControl = true
	a.mu.Unlock()
	return nil
}

// ListSessions returns the live windows of the commander tmux session.
func (a *App) ListSessions() []SessionInfo {
	ws, _ := a.host.List(a.cfg.TmuxSession)
	a.reconcile(ws)
	var out []SessionInfo
	for _, w := range ws {
		out = append(out, SessionInfo{WindowID: w.ID, Name: w.Name})
	}
	return out
}

// reconcile brings the in-memory registry in sync with the live tmux windows.
// The tmux `commander` session survives app restarts but the registry does not,
// so surviving windows would otherwise be unknown to SwapModel/SessionStats.
// For each window missing from the registry it creates a record, recovering the
// cwd from tmux and (best effort) the model from the window name (== model
// label at launch). Add-only: never prunes, to avoid racing a just-launched
// window that tmux hasn't listed yet.
func (a *App) reconcile(ws []tmux.WindowState) {
	// Forget windows that no longer exist. Ids are reused after a tmux server
	// restart, so without this a brand-new @1 could inherit a dead window's
	// model — and the file would grow for the life of the install.
	live := make([]string, 0, len(ws))
	for _, w := range ws {
		live = append(live, w.ID)
	}
	if _, err := a.windows.Prune(live); err != nil {
		a.reportError(fmt.Sprintf("could not prune window state: %v", err))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, w := range ws {
		if _, ok := a.sessions[w.ID]; ok {
			continue
		}
		rec := &sessionRec{Name: w.Name, Cwd: w.Cwd, LaunchedAt: a.now(), Status: "active"}
		known, _ := a.windows.Get(w.ID)
		// Prefer what was recorded at launch; fall back to the window name
		// (== label at launch) for windows launched before this file existed.
		for _, m := range a.cfg.Models {
			if m.ID == known.Model || (known.Model == "" && (m.Label == w.Name || m.ID == w.Name)) {
				rec.Model = m.ID
				rec.Provider = m.Provider
				break
			}
		}
		rec.RemoteControl = known.RemoteControl
		a.sessions[w.ID] = rec
	}
}

// SelectSession changes which session the single pane shows.
//
// It deliberately does NOT touch the session's Status. Status is "active" from
// launch until the Stop hook — it means "not yet finished", not "doing
// something" — so writing "active" here silently un-finished any session the
// user clicked on. The deck already clears its own finished flag in
// stores.svelte.js select().
func (a *App) SelectSession(windowID string) error {
	a.mu.Lock()
	a.current = windowID
	if r := a.sessions[windowID]; r != nil {
		r.AckMs = a.now().UnixMilli()
	}
	a.mu.Unlock()
	return nil
}

// KillSession terminates a session and forgets it.
func (a *App) KillSession(windowID string) error {
	if err := a.host.Kill(a.cfg.TmuxSession, windowID); err != nil {
		return err
	}
	a.mu.Lock()
	delete(a.sessions, windowID)
	if a.current == windowID {
		a.current = ""
	}
	a.mu.Unlock()
	_ = a.windows.Delete(windowID)
	return nil
}

// RenameSession renames a session's tmux window and registry name.
func (a *App) RenameSession(windowID, name string) error {
	if err := a.host.Rename(a.cfg.TmuxSession, windowID, name); err != nil {
		return err
	}
	a.mu.Lock()
	if r := a.sessions[windowID]; r != nil {
		r.Name = name
	}
	a.mu.Unlock()
	return nil
}

// SessionStats returns the card data for a session.
func (a *App) SessionStats(windowID string) SessionStats {
	a.mu.Lock()
	r := a.sessions[windowID]
	if r == nil {
		a.mu.Unlock()
		return SessionStats{}
	}
	model, provider, status := r.Model, r.Provider, r.Status
	cwd, tpath, launched := r.Cwd, r.TranscriptPath, r.LaunchedAt
	remoteControl := r.RemoteControl
	a.mu.Unlock()

	st := SessionStats{
		Model: model, Provider: provider, Status: status,
		UptimeSeconds: int(a.now().Sub(launched).Seconds()),
		RemoteControl: remoteControl,
		Cwd:           cwd,
	}
	if tpath == "" {
		if p, err := transcripts.NewestTranscript(a.projectsRoot(), cwd); err == nil {
			tpath = p
		}
	}
	if tpath != "" {
		st.ContextTokens, st.Turns = a.transcriptStats(tpath)
	}
	if m, ok := a.modelByID(model); ok {
		st.EstCostPerTurn = pricing.TurnInputCost(st.ContextTokens, m)
		st.Unpriced = m.Unpriced()
		// Pay-per-token sessions spend real money — band by cost. Subscription
		// sessions don't, so cost-red is noise; band by how full the context
		// window is instead. Ollama Cloud is routed AND subscription-billed,
		// which is why this is no longer an IsRouted() check. An unpriced model
		// (e.g. Zen/Bedrock cards, which always carry a $0 rate) has no
		// meaningful dollar band either — Band(0) would always read green — so
		// context fullness is the only real signal there too.
		if m.BandByContext() || m.Unpriced() {
			st.Band = pricing.ContextBand(st.ContextTokens)
		} else {
			st.Band = pricing.Band(st.EstCostPerTurn)
		}
	}
	return st
}

// transcriptStats returns a transcript's context tokens and turn count, reusing
// a cached parse while the file's mtime is unchanged. The stats poll hits every
// session every 5s, so this avoids re-parsing multi-MB transcripts each tick.
func (a *App) transcriptStats(path string) (int, int) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0
	}
	mod := fi.ModTime().UnixNano()
	a.statMu.Lock()
	if e, ok := a.statCache[path]; ok && e.mod == mod {
		a.statMu.Unlock()
		return e.ctx, e.turns
	}
	a.statMu.Unlock()

	ctx, _ := transcripts.ContextTokens(path)
	turns, _ := transcripts.TurnCount(path)
	a.statMu.Lock()
	if a.statCache == nil {
		a.statCache = map[string]statEntry{}
	}
	a.statCache[path] = statEntry{mod: mod, ctx: ctx, turns: turns}
	a.statMu.Unlock()
	return ctx, turns
}

// PickFolder opens a native directory picker and returns the chosen absolute
// path, or "" if the user cancels.
func (a *App) PickFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Choose project folder",
	})
}

// SwapModel changes a session's model in place: it kills the window and
// relaunches `claude --resume <id>` under the new model, same conversation.
// Everything that can fail (routed key, router startup) is checked BEFORE the
// old window is killed, so a failed swap never destroys the session.
func (a *App) SwapModel(windowID, newModelID string) (SessionInfo, error) {
	m, ok := a.modelByID(newModelID)
	if !ok {
		err := fmt.Errorf("unknown model %q", newModelID)
		a.reportError(err.Error())
		return SessionInfo{}, err
	}
	a.mu.Lock()
	old := a.sessions[windowID]
	if old == nil {
		a.mu.Unlock()
		return SessionInfo{}, fmt.Errorf("unknown session %q", windowID)
	}
	// Capture the fields while holding the lock; handleNotify writes this same
	// *sessionRec under a.mu, so reading them unlocked would be a data race.
	cwd, oldName, oldSID := old.Cwd, old.Name, old.ClaudeSessionID
	oldRC := old.RemoteControl
	a.mu.Unlock()

	// Pre-flight (before killing): routed models need a key + a healthy router.
	if m.IsRouted() {
		if ok, ref := modelReady(m); !ok {
			err := fmt.Errorf("model %q needs key: set %s in Providers", m.ID, ref)
			a.reportError(err.Error())
			return SessionInfo{}, err
		}
		if err := a.ensureRouter(); err != nil {
			a.reportError(fmt.Sprintf("could not start the model router: %v", err))
			return SessionInfo{}, err
		}
	}

	// Resolve the claude session id: registry, else newest transcript for cwd.
	sid := oldSID
	if sid == "" {
		if _, _, path, err := transcripts.StatsForCwd(a.projectsRoot(), cwd); err == nil {
			sid = strings.TrimSuffix(filepath.Base(path), ".jsonl")
		}
	}

	// Kill the old window and forget it.
	_ = a.host.Kill(a.cfg.TmuxSession, windowID)
	a.mu.Lock()
	delete(a.sessions, windowID)
	if a.current == windowID {
		a.current = ""
	}
	a.mu.Unlock()

	// Relaunch: resume the conversation if we have an id, else fresh.
	var extra []string
	if sid != "" {
		extra = []string{"--resume", sid}
	}
	// Carry Remote Control across the swap when the target is native; routed
	// targets can't bridge (RC refuses proxied BASE_URLs), so RC drops there.
	carryRC := oldRC && !m.IsRouted()
	if carryRC {
		name := oldName
		if name == "" {
			name = m.Label
		}
		extra = append(extra, "--remote-control", name)
	}
	info, err := a.startSession(cwd, m, extra)
	if err != nil {
		// The old window is already gone, but the conversation is safe on disk.
		werr := fmt.Errorf("swap relaunch failed (conversation preserved — relaunch %q to resume): %w", cwd, err)
		a.reportError(werr.Error())
		return SessionInfo{}, werr
	}
	// Preserve the user's display name.
	if oldName != "" {
		_ = a.host.Rename(a.cfg.TmuxSession, info.WindowID, oldName)
		a.mu.Lock()
		if r := a.sessions[info.WindowID]; r != nil {
			r.Name = oldName
		}
		a.mu.Unlock()
		info.Name = oldName
	}
	if carryRC {
		a.mu.Lock()
		if r := a.sessions[info.WindowID]; r != nil {
			r.RemoteControl = true
		}
		a.mu.Unlock()
		_ = a.windows.Update(info.WindowID, func(r *winstate.Record) { r.RemoteControl = true })
	}
	return info, nil
}
