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
	"sort"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/halalgami/CodingAgentCommander/internal/bedrock"
	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/hookmgr"
	"github.com/halalgami/CodingAgentCommander/internal/launch"
	"github.com/halalgami/CodingAgentCommander/internal/pricing"
	"github.com/halalgami/CodingAgentCommander/internal/ptybridge"
	"github.com/halalgami/CodingAgentCommander/internal/router"
	"github.com/halalgami/CodingAgentCommander/internal/secrets"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
	"github.com/halalgami/CodingAgentCommander/internal/transcripts"
	"github.com/halalgami/CodingAgentCommander/internal/wsterm"
)

// ModelInfo is the picker entry sent to the frontend.
type ModelInfo struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Routed bool   `json:"routed"`
	Ready  bool   `json:"ready"`
}

// SessionInfo describes a launched session for the sidebar.
type SessionInfo struct {
	WindowID string `json:"windowID"`
	Name     string `json:"name"`
	Model    string `json:"model"`
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
	// router is the lazily-started local LiteLLM proxy used for routed
	// (non-anthropic) models. Guarded by routerMu.
	router   *router.Controller
	routerMu sync.Mutex
	// routerHash fingerprints the resolved config (yaml+env) the running proxy
	// was started with, so ensureRouter can detect drift (a model or key added
	// after start) and restart instead of serving a stale model list.
	routerHash string
	// statCache memoizes parsed transcript stats keyed by path, invalidated by
	// mtime, so the 5s stats poll doesn't re-parse unchanged transcripts.
	statCache map[string]statEntry
	statMu    sync.Mutex
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
}

// SessionStats is the per-session card data.
type SessionStats struct {
	ContextTokens  int     `json:"contextTokens"`
	EstCostPerTurn float64 `json:"estCostPerTurn"`
	Band           string  `json:"band"`
	Turns          int     `json:"turns"`
	Model          string  `json:"model"`
	Provider       string  `json:"provider"`
	UptimeSeconds  int     `json:"uptimeSeconds"`
	Status         string  `json:"status"`
	RemoteControl  bool    `json:"remoteControl"`
	Cwd            string  `json:"cwd"`
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
	a := &App{host: tmux.NewExecHost(), sessions: map[string]*sessionRec{}, notifier: nativeNotifier{}}
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
	a.masterKey = randomHex(24)
	a.wsToken = randomHex(24)
	// Reap any litellm orphaned by a prior unclean exit before starting fresh.
	router.ReapStale(a.litellmConfigPath())
	a.router = router.NewController(0)
	a.startWS()
	_ = hookmgr.Install(a.settingsPath(), a.wsPort, a.wsToken)
	a.reconcile(a.reconcileWindows()) // adopt any windows surviving from a prior run
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

// snapshotModels returns a copy of the catalog under lock (safe to range lock-free).
func (a *App) snapshotModels() []config.Model {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]config.Model{}, a.cfg.Models...)
}

// modelByID looks up a model under lock.
func (a *App) modelByID(id string) (config.Model, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Model(id)
}

// Config returns all models for the picker (native + routed), each flagged
// Routed and Ready (key present for routed; native always ready).
func (a *App) Config() []ModelInfo {
	out := []ModelInfo{}
	for _, m := range a.snapshotModels() {
		ready, _ := modelReady(m)
		out = append(out, ModelInfo{ID: m.ID, Label: m.Label, Routed: m.IsRouted(), Ready: ready})
	}
	return out
}

// KeyInfo is a provider key slot for the admin panel.
type KeyInfo struct {
	Env      string `json:"env"`
	Set      bool   `json:"set"`
	Optional bool   `json:"optional"`
}

// KeyStatus lists the distinct credential slots across routed models (required
// first, then optional) with whether each is set.
func (a *App) KeyStatus() []KeyInfo {
	seen := map[string]bool{}
	out := []KeyInfo{}
	add := func(ref string, optional bool) {
		if seen[ref] {
			return
		}
		seen[ref] = true
		v, err := secrets.Get(ref)
		out = append(out, KeyInfo{Env: ref, Set: err == nil && v != "", Optional: optional})
	}
	models := a.snapshotModels()
	for _, m := range models {
		for _, ref := range m.CredEnvs() {
			add(ref, false)
		}
	}
	for _, m := range models {
		for _, ref := range m.OptionalCredEnvs() {
			add(ref, true)
		}
	}
	// AWS creds must be enterable BEFORE any bedrock model exists: the 🔍
	// Discover flow needs them to list models, but the loops above only
	// surface envs of already-configured models (chicken-and-egg). Always
	// list the AWS trio; optional until a bedrock model is in the catalog.
	hasBedrock := false
	for _, m := range models {
		if m.Provider == config.ProviderBedrock {
			hasBedrock = true
			break
		}
	}
	add(config.AWSAccessKeyEnv, !hasBedrock)
	add(config.AWSSecretKeyEnv, !hasBedrock)
	add(config.AWSSessionTokenEnv, true)
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

// AddModel validates and appends a catalog model, persisting before it is
// committed to the in-memory catalog.
func (a *App) AddModel(in ModelInput) error {
	in.ID = strings.TrimSpace(in.ID)
	in.Provider = strings.TrimSpace(in.Provider)
	if in.ID == "" || in.Provider == "" {
		return fmt.Errorf("id and provider are required")
	}
	switch in.Provider {
	case config.ProviderAnthropic:
		in.Upstream, in.APIBase, in.KeyEnv, in.Region = "", "", "", "" // native carries no routed fields
	case config.ProviderBedrock:
		// Credentials come from the shared AWS_* keychain refs (set in Providers),
		// so no key_env/api_base — only the bedrock/ model string and a region.
		if in.Upstream == "" || in.Region == "" {
			return fmt.Errorf("bedrock models require upstream (bedrock/<model-id>) and region")
		}
		in.Upstream = config.NormalizeBedrockUpstream(strings.TrimSpace(in.Upstream))
		in.APIBase, in.KeyEnv = "", ""
	default:
		if in.Upstream == "" || in.APIBase == "" || in.KeyEnv == "" {
			return fmt.Errorf("routed models require upstream, api_base, and key_env")
		}
		in.Region = ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
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
		ModelID: m.ID,
	})
	if err != nil {
		return SessionInfo{}, err
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
		return SessionInfo{}, fmt.Errorf("unknown model %q", modelID)
	}
	var extra []string
	if remoteControl {
		if m.IsRouted() {
			return SessionInfo{}, fmt.Errorf("remote control needs a native Anthropic session")
		}
		name := m.Label
		if name == "" {
			name = m.ID
		}
		extra = []string{"--remote-control", name}
	}
	s, err := a.startSession(folder, m, extra)
	if err == nil && remoteControl {
		a.mu.Lock()
		if rec := a.sessions[s.WindowID]; rec != nil {
			rec.RemoteControl = true
		}
		a.mu.Unlock()
		_ = a.host.SetOption(s.WindowID, "@commander_rc", "1")
	}
	return s, err
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
	_ = a.host.SetOption(windowID, "@commander_rc", "1") // best-effort durable marker
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
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, w := range ws {
		if _, ok := a.sessions[w.ID]; ok {
			continue
		}
		rec := &sessionRec{Name: w.Name, Cwd: w.Cwd, LaunchedAt: a.now(), Status: "active"}
		// Prefer the durable @commander_model marker; fall back to the window
		// name (== label at launch) only for windows from before markers existed.
		for _, m := range a.cfg.Models {
			if m.ID == w.ModelID || (w.ModelID == "" && (m.Label == w.Name || m.ID == w.Name)) {
				rec.Model = m.ID
				rec.Provider = m.Provider
				break
			}
		}
		if v, _ := a.host.GetOption(w.ID, "@commander_rc"); v == "1" {
			rec.RemoteControl = true
		}
		a.sessions[w.ID] = rec
	}
}

// SelectSession changes which session the single pane shows.
func (a *App) SelectSession(windowID string) error {
	a.mu.Lock()
	a.current = windowID
	if r := a.sessions[windowID]; r != nil {
		r.Status = "active"
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
	return nil
}

// RenameSession renames a session's tmux window and registry name.
func (a *App) RenameSession(windowID, name string) error {
	if err := a.host.Rename(windowID, name); err != nil {
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
		// Routed sessions spend real per-token money — band by cost. Native
		// subscription sessions don't, so cost-red is noise; band by how full
		// the context window is instead.
		if m.IsRouted() {
			st.Band = pricing.Band(st.EstCostPerTurn)
		} else {
			st.Band = pricing.ContextBand(st.ContextTokens)
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
		return SessionInfo{}, fmt.Errorf("unknown model %q", newModelID)
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
			return SessionInfo{}, fmt.Errorf("model %q needs key: set %s in Providers", m.ID, ref)
		}
		if err := a.ensureRouter(); err != nil {
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
		return SessionInfo{}, fmt.Errorf("swap relaunch failed (conversation preserved — relaunch %q to resume): %w", cwd, err)
	}
	// Preserve the user's display name.
	if oldName != "" {
		_ = a.host.Rename(info.WindowID, oldName)
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
		_ = a.host.SetOption(info.WindowID, "@commander_rc", "1")
	}
	return info, nil
}
