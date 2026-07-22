package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/halalgami/CodingAgentCommander/internal/config"
	"github.com/halalgami/CodingAgentCommander/internal/secrets"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
)

func TestBuildLaunchSpecNative(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatalf("load: %v", err)
	}
	models := a.Config()
	if len(models) == 0 {
		t.Fatal("expected at least one model in picker")
	}
	// B2b: the picker includes both native and routed models, each flagged.
	var sawNative, sawRouted bool
	for _, m := range models {
		if m.Routed {
			sawRouted = true
		} else {
			sawNative = true
			if !m.Ready {
				t.Errorf("native model %s should always be Ready", m.ID)
			}
		}
	}
	if !sawNative {
		t.Error("expected at least one native model in picker")
	}
	if !sawRouted {
		t.Error("expected at least one routed model in picker (B2b)")
	}
}

func TestRegistryAndStats(t *testing.T) {
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	// Seed a session record directly (avoid spawning tmux/claude here).
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "proj", Cwd: "/tmp/x", Model: "claude-opus-4-8", Provider: "anthropic",
			LaunchedAt: a.now(), Status: "active"},
	}
	st := a.SessionStats("@1")
	if st.Model != "claude-opus-4-8" || st.Provider != "anthropic" || st.Status != "active" {
		t.Errorf("stats basics wrong: %+v", st)
	}
	if st.UptimeSeconds < 0 {
		t.Errorf("uptime negative")
	}
	// Unknown window → zero-value, no panic.
	_ = a.SessionStats("@nope")
}

// fakeNotifier/fakeEmitter are shared test doubles. They carry their own
// mutex because TestSessionStatsHandleNotifyNoRace drives handleNotify (and
// therefore these methods) from many goroutines concurrently; without
// synchronization here, -race flags a race in the double itself rather than
// in the production code under test.
type fakeNotifier struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeNotifier) Notify(title, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}

type fakeEmitter struct {
	mu     sync.Mutex
	events []string
}

func (f *fakeEmitter) Emit(event string, data ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func TestNotifyBackgroundVsFocused(t *testing.T) {
	a := NewApp()
	fn := &fakeNotifier{}
	fe := &fakeEmitter{}
	a.notifier = fn
	a.emitter = fe
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "bg", Cwd: "/tmp/a", Status: "active"},
		"@2": {Name: "fg", Cwd: "/tmp/b", Status: "active"},
	}
	a.current = "@2" // @2 is focused

	// Finish on the BACKGROUND session (@1, cwd /tmp/a) -> notify + event.
	a.handleNotify([]byte(`{"session_id":"s1","cwd":"/tmp/a","transcript_path":"/tmp/a/s1.jsonl","hook_event_name":"Stop"}`))
	if fn.calls != 1 {
		t.Errorf("expected 1 desktop notify for background, got %d", fn.calls)
	}
	if len(fe.events) != 1 || fe.events[0] != "session:finished" {
		t.Errorf("expected session:finished event, got %v", fe.events)
	}
	if a.sessions["@1"].Status != "finished" {
		t.Errorf("bg session status = %q", a.sessions["@1"].Status)
	}
	if a.sessions["@1"].TranscriptPath == "" {
		t.Error("transcript path not recorded from payload")
	}

	// Finish on the FOCUSED session (@2, cwd /tmp/b) -> NO desktop notify.
	a.handleNotify([]byte(`{"session_id":"s2","cwd":"/tmp/b","transcript_path":"/tmp/b/s2.jsonl","hook_event_name":"Stop"}`))
	if fn.calls != 1 {
		t.Errorf("focused session must not desktop-notify; calls=%d", fn.calls)
	}
}

func TestSessionStatsHandleNotifyNoRace(t *testing.T) {
	a := NewApp()
	a.notifier = &fakeNotifier{}
	a.emitter = &fakeEmitter{}
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "x", Cwd: "/tmp/z", Model: "claude-opus-4-8", Provider: "anthropic", LaunchedAt: a.now(), Status: "active"},
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = a.SessionStats("@1") }()
		go func() {
			defer wg.Done()
			a.handleNotify([]byte(`{"session_id":"s","cwd":"/tmp/z","transcript_path":"/tmp/z/s.jsonl"}`))
		}()
	}
	wg.Wait()
}

func TestTranscriptStatsCache(t *testing.T) {
	a := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	line := `{"type":"assistant","message":{"usage":{"input_tokens":100,"cache_read_input_tokens":50,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(line+line), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, turns := a.transcriptStats(path)
	if ctx != 150 || turns != 2 {
		t.Fatalf("ctx=%d turns=%d, want 150/2", ctx, turns)
	}
	// Second call at the same mtime must hit the cache and agree.
	if c2, t2 := a.transcriptStats(path); c2 != 150 || t2 != 2 {
		t.Errorf("cached read disagreed: ctx=%d turns=%d", c2, t2)
	}
	if _, ok := a.statCache[path]; !ok {
		t.Error("stat cache not populated")
	}
	// A missing file yields zeros, not a panic.
	if c, tn := a.transcriptStats(filepath.Join(dir, "nope.jsonl")); c != 0 || tn != 0 {
		t.Errorf("missing file should be 0/0, got %d/%d", c, tn)
	}
}

func TestHashConfig(t *testing.T) {
	base := hashConfig([]byte("yaml-a"), []string{"K=1", "J=2"})
	// Env order must not matter (routerConfigAndEnv builds it in catalog order).
	if base != hashConfig([]byte("yaml-a"), []string{"J=2", "K=1"}) {
		t.Error("hash must be order-independent over env")
	}
	// A changed model list (yaml) must change the hash → triggers a restart.
	if base == hashConfig([]byte("yaml-b"), []string{"K=1", "J=2"}) {
		t.Error("different yaml must produce a different hash")
	}
	// A newly-resolved key (new env entry) must change the hash.
	if base == hashConfig([]byte("yaml-a"), []string{"K=1", "J=2", "L=3"}) {
		t.Error("added key must produce a different hash")
	}
}

// TestEnsureRouterDetectsDrift proves the resolved config the restart decision
// keys on actually changes when a routed model's key becomes available — the
// Bedrock-404-after-first-start case. It compares hashes rather than launching
// litellm (that path is exercised by the router package's health test).
func TestEnsureRouterDetectsDrift(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	a.masterKey = "k"
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	y1, e1, _, _ := a.routerConfigAndEnv()
	before := hashConfig(y1, e1)
	// Add the AWS creds that were missing at first start.
	_ = secrets.Set(config.AWSAccessKeyEnv, "AKIA")
	_ = secrets.Set(config.AWSSecretKeyEnv, "secret")
	y2, e2, _, _ := a.routerConfigAndEnv()
	after := hashConfig(y2, e2)
	if before == after {
		t.Error("resolving new keys must change the config hash so ensureRouter restarts")
	}
	if !strings.Contains(string(y2), "bedrock/") {
		t.Error("bedrock models should now be in the yaml")
	}
}

func TestRouterConfigAndEnv(t *testing.T) {
	keyring.MockInit()
	_ = secrets.Set("ZEN_KEY", "sk-zen-xyz")
	// example.config.toml also carries bedrock models; provide the shared AWS
	// creds so nothing is reported missing.
	_ = secrets.Set(config.AWSAccessKeyEnv, "AKIA-test")
	_ = secrets.Set(config.AWSSecretKeyEnv, "aws-secret-test")

	a := NewApp()
	a.masterKey = "sk-master"
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	yaml, env, missing, err := a.routerConfigAndEnv()
	if err != nil {
		t.Fatalf("routerConfigAndEnv: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing keys, got %v", missing)
	}
	if !strings.Contains(string(yaml), "os.environ/ZEN_KEY") || !strings.Contains(string(yaml), "sk-master") {
		t.Errorf("yaml missing key ref or master key: %s", yaml)
	}
	// Bedrock models resolve to bedrock/ upstreams with AWS cred refs (no api_key).
	if !strings.Contains(string(yaml), "bedrock/") ||
		!strings.Contains(string(yaml), "os.environ/"+config.AWSAccessKeyEnv) {
		t.Errorf("yaml missing bedrock model or AWS cred ref: %s", yaml)
	}
	joined := strings.Join(env, " ")
	if !strings.Contains(joined, "ZEN_KEY=sk-zen-xyz") ||
		!strings.Contains(joined, config.AWSAccessKeyEnv+"=AKIA-test") {
		t.Errorf("env missing resolved key(s): %v", env)
	}
}

func TestRouterConfigAndEnvReportsMissing(t *testing.T) {
	keyring.MockInit() // empty keychain
	a := NewApp()
	a.masterKey = "k"
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	_, _, missing, err := a.routerConfigAndEnv()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range missing {
		if m == "ZEN_KEY" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ZEN_KEY in missing, got %v", missing)
	}
}

func TestKeyBindingsAndReady(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	// Initially the Zen key is unset → its model is not ready.
	models := a.Config()
	var zen *ModelInfo
	for i := range models {
		if models[i].ID == "kimi-k2.7-code" {
			zen = &models[i]
		}
	}
	if zen == nil {
		t.Fatal("routed model kimi-k2.7-code not in picker (routed models must be shown)")
	}
	if zen.Ready {
		t.Error("kimi-k2.7-code should not be ready with no key")
	}
	// KeyStatus lists ZEN_KEY as unset.
	if st := a.KeyStatus(); len(st) == 0 || st[0].Set {
		t.Errorf("expected an unset key in KeyStatus, got %v", st)
	}
	// Set it → ready flips.
	if err := a.SetKey("ZEN_KEY", "sk-zen"); err != nil {
		t.Fatal(err)
	}
	for _, m := range a.Config() {
		if m.ID == "kimi-k2.7-code" && !m.Ready {
			t.Error("kimi-k2.7-code should be ready after SetKey")
		}
	}
	// Clear → unset again.
	if err := a.ClearKey("ZEN_KEY"); err != nil {
		t.Fatal(err)
	}
	if v, _ := secrets.Get("ZEN_KEY"); v != "" {
		t.Error("key not cleared")
	}
}

func TestLaunchRoutedMissingKeyErrorsEarly(t *testing.T) {
	keyring.MockInit() // no keys
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	// kimi-k2.7-code is routed and has no key → must error WITHOUT starting the router
	// or launching tmux.
	_, err := a.LaunchSession(t.TempDir(), "kimi-k2.7-code", false)
	if err == nil {
		t.Fatal("expected error launching routed model with no key")
	}
	if a.router != nil && a.router.Running() {
		t.Error("router must not start when required key is missing")
	}
	if !strings.Contains(err.Error(), "ZEN_KEY") {
		t.Errorf("error should name the missing key_env, got %v", err)
	}
}

func TestRouterConfigAndEnvSkipsUnconfiguredProviders(t *testing.T) {
	keyring.MockInit()
	_ = secrets.Set("ZEN_KEY", "sk-zen")
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.toml")
	os.WriteFile(cfgPath, []byte(`
default_model = "m-ready"
[[models]]
id = "m-ready"
provider = "zen"
upstream = "openai/m-ready"
api_base = "https://x/v1"
key_env = "ZEN_KEY"
[[models]]
id = "m-missing"
provider = "zen"
upstream = "openai/m-missing"
api_base = "https://x/v1"
key_env = "OTHER_KEY"
`), 0o600)
	a := NewApp()
	a.masterKey = "k"
	if err := a.loadConfigFrom(cfgPath); err != nil {
		t.Fatal(err)
	}
	yaml, env, missing, err := a.routerConfigAndEnv()
	if err != nil {
		t.Fatal(err)
	}
	s := string(yaml)
	if !strings.Contains(s, "m-ready") {
		t.Error("ready model missing from yaml")
	}
	if strings.Contains(s, "m-missing") {
		t.Error("unconfigured model must be excluded from yaml")
	}
	if strings.Join(env, " ") != "ZEN_KEY=sk-zen" {
		t.Errorf("env=%v", env)
	}
	found := false
	for _, m := range missing {
		if m == "OTHER_KEY" {
			found = true
		}
	}
	if !found {
		t.Errorf("OTHER_KEY should be reported missing, got %v", missing)
	}
}

func TestCatalogAddRemove(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.toml")
	os.WriteFile(cfgPath, []byte(`
default_model = "claude-opus-4-8"
[[models]]
id = "claude-opus-4-8"
label = "Opus"
provider = "anthropic"
`), 0o600)

	a := NewApp()
	if err := a.loadConfigFrom(cfgPath); err != nil {
		t.Fatal(err)
	}
	if err := a.AddProvider(config.ProviderOpencodeGo, "", ""); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	// Add a routed model.
	if err := a.AddModel(ModelInput{ID: "kimi", Label: "Kimi", Provider: config.ProviderOpencodeGo,
		Upstream: "openai/kimi"}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	// It's in the in-memory catalog AND persisted.
	if _, ok := a.cfg.Model("kimi"); !ok {
		t.Error("model not added in-memory")
	}
	reloaded, _ := configLoad(cfgPath)
	if _, ok := reloaded.Model("kimi"); !ok {
		t.Error("model not persisted to disk")
	}
	// Validation: dup id, and routed without upstream.
	if err := a.AddModel(ModelInput{ID: "kimi", Provider: config.ProviderOpencodeGo, Upstream: "x"}); err == nil {
		t.Error("expected dup-id rejection")
	}
	if err := a.AddModel(ModelInput{ID: "bad", Provider: config.ProviderOpencodeGo}); err == nil {
		t.Error("expected routed-without-upstream rejection")
	}
	// Remove.
	if err := a.RemoveModel("kimi"); err != nil {
		t.Fatalf("RemoveModel: %v", err)
	}
	if _, ok := a.cfg.Model("kimi"); ok {
		t.Error("model not removed")
	}
	// Cannot remove the default model.
	if err := a.RemoveModel("claude-opus-4-8"); err == nil {
		t.Error("expected refusal to remove default_model")
	}
}

func configLoad(p string) (config.Config, error) { return config.Load(p) }

func TestCatalogConcurrentReadWriteNoRace(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.toml")
	os.WriteFile(cfgPath, []byte("default_model = \"a\"\n[[models]]\nid = \"a\"\nprovider = \"anthropic\"\n"), 0o600)
	a := NewApp()
	if err := a.loadConfigFrom(cfgPath); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); _ = a.Config() }()
		go func() { defer wg.Done(); _ = a.KeyStatus() }()
		go func(n int) {
			defer wg.Done()
			_ = a.AddModel(ModelInput{ID: fmt.Sprintf("m%d", n), Provider: "anthropic"})
		}(i)
	}
	wg.Wait()
}

// captureHost is a fake tmux.Host for tests: it records Launch/Kill/SendKeys
// calls instead of shelling out to real tmux.
type captureHost struct {
	mu       sync.Mutex
	launched []tmux.LaunchSpec
	killed   []string
	windows  []tmux.WindowState
	sentKeys []string // windowID+"\x00"+text, one per SendKeys call
}

func (c *captureHost) Launch(s tmux.LaunchSpec) (tmux.WindowState, error) {
	c.mu.Lock()
	c.launched = append(c.launched, s)
	c.mu.Unlock()
	return tmux.WindowState{ID: "@sw", Name: s.WindowName}, nil
}
func (c *captureHost) List(string) ([]tmux.WindowState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.windows, nil
}
func (c *captureHost) Kill(_, id string) error {
	c.mu.Lock()
	c.killed = append(c.killed, id)
	c.mu.Unlock()
	return nil
}
func (c *captureHost) Rename(string, string) error { return nil }
func (c *captureHost) SendKeys(windowID, text string) error {
	c.mu.Lock()
	c.sentKeys = append(c.sentKeys, windowID+"\x00"+text)
	c.mu.Unlock()
	return nil
}
func (c *captureHost) SetOption(string, string, string) error   { return nil }
func (c *captureHost) GetOption(string, string) (string, error) { return "", nil }

func TestStartSessionNative(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	m, _ := a.cfg.Model("claude-opus-4-8")
	info, err := a.startSession("/tmp/p", m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.launched) != 1 {
		t.Fatalf("expected 1 launch, got %d", len(ch.launched))
	}
	spec := ch.launched[0]
	if len(spec.Command) != 1 || spec.Command[0] != "claude" {
		t.Errorf("cmd=%v", spec.Command)
	}
	if spec.Env["ANTHROPIC_MODEL"] != "claude-opus-4-8" {
		t.Errorf("env=%v", spec.Env)
	}
	if spec.ModelID != "claude-opus-4-8" {
		t.Errorf("durable model marker not set: ModelID=%q", spec.ModelID)
	}
	if _, ok := a.sessions[info.WindowID]; !ok {
		t.Error("not recorded")
	}
}

func TestSwapRoutedMissingKeyDoesNotKill(t *testing.T) {
	keyring.MockInit() // no keys
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	a.sessions = map[string]*sessionRec{"@1": {Name: "s", Cwd: "/tmp/z", Model: "claude-opus-4-8", Provider: "anthropic", Status: "active"}}
	a.current = "@1"
	// swap to a routed model with no key -> error, and the old window must NOT be killed.
	if _, err := a.SwapModel("@1", "glm-5.2"); err == nil {
		t.Fatal("expected error swapping to routed model with no key")
	}
	if len(ch.killed) != 0 {
		t.Errorf("old window killed on failed swap: %v", ch.killed)
	}
	if _, ok := a.sessions["@1"]; !ok {
		t.Error("session removed on failed swap")
	}
}

func TestSwapNativeResumesById(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	a.sessions = map[string]*sessionRec{"@1": {Name: "myproj", Cwd: "/tmp/z", Model: "claude-opus-4-8", Provider: "anthropic", ClaudeSessionID: "sid-123", Status: "active"}}
	a.current = "@1"
	info, err := a.SwapModel("@1", "claude-sonnet-5")
	if err != nil {
		t.Fatalf("SwapModel: %v", err)
	}
	if len(ch.killed) != 1 || ch.killed[0] != "@1" {
		t.Errorf("old window not killed: %v", ch.killed)
	}
	if len(ch.launched) != 1 {
		t.Fatalf("expected 1 relaunch, got %d", len(ch.launched))
	}
	cmd := ch.launched[0].Command
	if len(cmd) != 3 || cmd[0] != "claude" || cmd[1] != "--resume" || cmd[2] != "sid-123" {
		t.Errorf("resume command wrong: %v", cmd)
	}
	if ch.launched[0].Env["ANTHROPIC_MODEL"] != "claude-sonnet-5" {
		t.Errorf("new model env wrong: %v", ch.launched[0].Env)
	}
	if info.Name != "myproj" {
		t.Errorf("name not preserved: %q", info.Name)
	}
}

func TestSwapHandleNotifyNoRace(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	a.host = &captureHost{}
	a.notifier = &fakeNotifier{}
	a.emitter = &fakeEmitter{}
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "s", Cwd: "/tmp/z", Model: "claude-opus-4-8", Provider: "anthropic", Status: "active"},
	}
	a.current = "@1"
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			a.handleNotify([]byte(`{"session_id":"s","cwd":"/tmp/z","transcript_path":"/tmp/z/s.jsonl"}`))
		}()
		go func() {
			defer wg.Done()
			_, _ = a.SwapModel("@1", "claude-sonnet-5")
		}()
	}
	wg.Wait()
}

func TestReconcileAdoptsOrphanWindows(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	// A window surviving from a prior run: in tmux, not in the registry.
	// Its name equals a catalog model label so the model is recovered.
	label := ""
	for _, m := range a.cfg.Models {
		if m.ID == "claude-opus-4-8" {
			label = m.Label
		}
	}
	a.host = &captureHost{windows: []tmux.WindowState{{ID: "@0", Name: label, Cwd: "/tmp/proj"}}}

	// Before reconcile the registry is empty -> SwapModel would say "unknown session".
	a.reconcile(a.reconcileWindows())

	a.mu.Lock()
	r := a.sessions["@0"]
	a.mu.Unlock()
	if r == nil {
		t.Fatal("orphan window @0 not adopted")
	}
	if r.Cwd != "/tmp/proj" {
		t.Errorf("cwd not recovered: %q", r.Cwd)
	}
	if r.Model != "claude-opus-4-8" {
		t.Errorf("model not recovered from window name: %q", r.Model)
	}
}

func TestEnableRemoteControlNativeSendsSlashCommand(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	s, err := a.LaunchSession(t.TempDir(), "claude-opus-4-8", false)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if err := a.EnableRemoteControl(s.WindowID); err != nil {
		t.Fatalf("EnableRemoteControl: %v", err)
	}
	want := s.WindowID + "\x00" + "/remote-control " + s.Name
	if len(ch.sentKeys) == 0 || ch.sentKeys[len(ch.sentKeys)-1] != want {
		t.Fatalf("send-keys not recorded, got %v (want %q)", ch.sentKeys, want)
	}
	st := a.SessionStats(s.WindowID)
	if !st.RemoteControl {
		t.Fatal("stats should report remoteControl=true")
	}
}

// TestEnableRemoteControlRejectsRouted seeds a routed session record directly
// (rather than going through LaunchSession, which would need a live litellm
// router + a real key — see TestSwapRoutedMissingKeyDoesNotKill for the same
// pattern) to exercise EnableRemoteControl's provider check in isolation.
func TestEnableRemoteControlRejectsRouted(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	a.host = &captureHost{}
	a.sessions = map[string]*sessionRec{
		"@1": {Name: "go-session", Cwd: "/tmp/z", Model: "kimi-k2.7-code", Provider: "opencode-go",
			LaunchedAt: a.now(), Status: "active"},
	}
	if err := a.EnableRemoteControl("@1"); err == nil {
		t.Fatal("expected error for routed session")
	}
}

func TestLaunchSessionRemoteControlFlagAppendsArg(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	if _, err := a.LaunchSession(t.TempDir(), "claude-opus-4-8", true); err != nil {
		t.Fatalf("launch: %v", err)
	}
	if len(ch.launched) != 1 {
		t.Fatalf("expected 1 launch, got %d", len(ch.launched))
	}
	cmd := ch.launched[0].Command
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "--remote-control") {
		t.Fatalf("claude args missing --remote-control: %v", cmd)
	}
}

func TestLaunchSessionRemoteControlRejectsRouted(t *testing.T) {
	keyring.MockInit() // no keys — must fail on the routed+RC check, not the missing-key check
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	a.host = &captureHost{}
	if _, err := a.LaunchSession(t.TempDir(), "kimi-k2.7-code", true); err == nil {
		t.Fatal("expected pre-launch error for routed + RC")
	}
}

// A native→native swap must re-enable Remote Control on the relaunched
// session (the old process — and its RC bridge — dies with the window).
func TestSwapCarriesRemoteControlToNativeTarget(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	if err := a.loadConfigFrom("example.config.toml"); err != nil {
		t.Fatal(err)
	}
	ch := &captureHost{}
	a.host = ch
	s, err := a.LaunchSession(t.TempDir(), "claude-opus-4-8", true)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	info, err := a.SwapModel(s.WindowID, "claude-sonnet-5")
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	last := ch.launched[len(ch.launched)-1]
	joined := strings.Join(last.Command, " ")
	if !strings.Contains(joined, "--remote-control") {
		t.Fatalf("swap relaunch args missing --remote-control: %v", last.Command)
	}
	if st := a.SessionStats(info.WindowID); !st.RemoteControl {
		t.Fatal("swapped session should keep remoteControl=true")
	}
}

// newProviderTestApp builds an App with a fresh keyring and config.Default(),
// backed by a scratch configPath, for provider-resolution tests.
func newProviderTestApp(t *testing.T) *App {
	t.Helper()
	keyring.MockInit()
	a := NewApp()
	a.configPath = filepath.Join(t.TempDir(), "config.toml")
	a.cfg = config.Default()
	return a
}

func TestSnapshotModelsResolvesProviderFields(t *testing.T) {
	a := newProviderTestApp(t)
	a.cfg = config.Config{
		Providers: []config.Provider{{Type: config.ProviderOpencodeGo, APIBase: config.ZenDefaultAPIBase}},
		Models: []config.Model{
			{ID: "glm", Provider: config.ProviderOpencodeGo, Upstream: "openai/glm-5.2"},
		},
	}
	m := a.snapshotModels()[0]
	if m.APIBase != config.ZenDefaultAPIBase || m.KeyEnv != config.ZenKeyEnv {
		t.Fatalf("snapshotModels must resolve: %+v", m)
	}
	got, ok := a.modelByID("glm")
	if !ok || got.KeyEnv != config.ZenKeyEnv {
		t.Fatalf("modelByID must resolve: %+v %v", got, ok)
	}
}

func TestKeyStatusDerivesFromProviders(t *testing.T) {
	a := newProviderTestApp(t)
	a.cfg = config.Config{Models: []config.Model{{ID: "a", Provider: config.ProviderAnthropic}}}
	if got := a.KeyStatus(); len(got) != 0 {
		t.Fatalf("no defined providers -> no slots, got %+v", got)
	}
	a.cfg.Providers = []config.Provider{{Type: config.ProviderOpencodeGo, APIBase: config.ZenDefaultAPIBase}}
	got := a.KeyStatus()
	if len(got) != 1 || got[0].Env != config.ZenKeyEnv || got[0].Optional {
		t.Fatalf("zen defined -> ZEN_KEY required slot, got %+v", got)
	}
	a.cfg.Providers = append(a.cfg.Providers, config.Provider{Type: config.ProviderBedrock, Region: "us-east-1"})
	envs := map[string]bool{}
	for _, k := range a.KeyStatus() {
		envs[k.Env] = k.Optional
	}
	if len(envs) != 4 {
		t.Fatalf("want ZEN + AWS trio, got %+v", envs)
	}
	if envs[config.AWSAccessKeyEnv] || envs[config.AWSSecretKeyEnv] || !envs[config.AWSSessionTokenEnv] {
		t.Fatalf("AWS key+secret required, session token optional: %+v", envs)
	}
}

func TestAddRemoveProvider(t *testing.T) {
	a := newProviderTestApp(t)
	if err := a.AddProvider("nonsense", "", ""); err == nil {
		t.Fatal("unknown type must fail")
	}
	if err := a.AddProvider(config.ProviderOpencodeGo, "", ""); err != nil {
		t.Fatal(err)
	}
	if p, ok := a.cfg.ProviderByType(config.ProviderOpencodeGo); !ok || p.APIBase != config.ZenDefaultAPIBase {
		t.Fatalf("empty api_base must default: %+v", p)
	}
	if err := a.AddProvider(config.ProviderOpencodeGo, "", ""); err == nil {
		t.Fatal("duplicate must fail")
	}
	// RemoveProvider drops the entry and its models.
	a.cfg.Models = append(a.cfg.Models, config.Model{ID: "glm", Provider: config.ProviderOpencodeGo, Upstream: "openai/glm-5.2"})
	if err := a.RemoveProvider(config.ProviderOpencodeGo); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.cfg.ProviderByType(config.ProviderOpencodeGo); ok {
		t.Fatal("provider entry must be gone")
	}
	if _, ok := a.cfg.Model("glm"); ok {
		t.Fatal("provider's models must be gone")
	}
	// Refuse removal that would delete the default model.
	a.cfg.DefaultModel = "glm2"
	a.cfg.Providers = []config.Provider{{Type: config.ProviderOpencodeGo, APIBase: config.ZenDefaultAPIBase}}
	a.cfg.Models = append(a.cfg.Models, config.Model{ID: "glm2", Provider: config.ProviderOpencodeGo, Upstream: "openai/glm-5.2"})
	if err := a.RemoveProvider(config.ProviderOpencodeGo); err == nil {
		t.Fatal("must refuse removing provider of default model")
	}
}

// A missing config file is first-run: Commander must seed the starter catalog
// and boot launchable (the public README promises zero-config first launch).
func TestLoadConfigMissingFileSeedsDefaults(t *testing.T) {
	keyring.MockInit()
	a := NewApp()
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := a.loadConfigFrom(path); err != nil {
		t.Fatalf("first-run load: %v", err)
	}
	if len(a.cfg.Models) == 0 {
		t.Fatal("seeded config has no models")
	}
	if a.configPath != path {
		t.Fatalf("configPath not set: %q", a.configPath)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	// AddModel must persist against the seeded path (the old bug: empty
	// configPath made Save rename into "").
	if err := a.AddModel(ModelInput{ID: "x-test", Provider: "anthropic", Label: "X"}); err != nil {
		t.Fatalf("AddModel after seed: %v", err)
	}
}

func TestAddModelRequiresDefinedProviderAndPersistsClean(t *testing.T) {
	a := newProviderTestApp(t)
	err := a.AddModel(ModelInput{ID: "glm", Provider: config.ProviderOpencodeGo, Upstream: "openai/glm-5.2"})
	if err == nil || !strings.Contains(err.Error(), "define") {
		t.Fatalf("undefined provider must be rejected with guidance, got %v", err)
	}
	if err := a.AddProvider(config.ProviderOpencodeGo, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.AddModel(ModelInput{ID: "glm", Provider: config.ProviderOpencodeGo, Upstream: "openai/glm-5.2"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := a.cfg.Model("glm")
	if raw.KeyEnv != "" || raw.APIBase != "" {
		t.Fatalf("persisted model must not carry inline provider fields: %+v", raw)
	}
	resolved, _ := a.modelByID("glm")
	if resolved.KeyEnv != config.ZenKeyEnv || resolved.APIBase != config.ZenDefaultAPIBase {
		t.Fatalf("resolution must supply fields: %+v", resolved)
	}
}
