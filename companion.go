package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"
)

// companionSnapshotCallsForTest counts calls to companionSnapshot. It exists
// solely so pushCompanion's "exactly one snapshot per push" invariant is
// provable in a test: companionSnapshot takes a.mu and sorts every session,
// so calling it twice per push is not just wasted work — a concurrent finish
// landing between the two calls would feed the overlay and the deck two
// DIFFERENT states for what the caller intended as one atomic push.
var companionSnapshotCallsForTest atomic.Int64

// companionState is the FLAT reaction-engine payload the overlay page has
// always polled. It is kept byte-compatible on the wire and is now derived
// from CompanionState rather than computed independently.
type companionState struct {
	Running   int  `json:"running"`
	Finished  int  `json:"finished"`
	Awaiting  bool `json:"awaiting"`
	Error     bool `json:"error"`
	QuotaHigh bool `json:"quotaHigh"`
	// FinishSeq is a monotonic counter bumped once per Stop-hook finish
	// (handleNotify). The overlay treats an increment as "a new session just
	// finished". LastFinished carries that session's project name.
	FinishSeq    int    `json:"finishSeq"`
	LastFinished string `json:"lastFinished"`
}

// SessionMood is one session's companion-facing state. Status carries only the
// two values the code actually assigns — handleNotify writes "finished" and
// startSession/reconcile write "active". There is no "error" or "idle" status
// (§2.2 defect 11); liveness comes from last output, frontend-side (§6.3).
type SessionMood struct {
	WindowID      string `json:"windowID"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	StatusSinceMs int64  `json:"statusSinceMs"`
	LastFinishMs  int64  `json:"lastFinishMs"`
	ErrorMs       int64  `json:"errorMs"`
	// AckMs is the ms-epoch the user last selected (acknowledged) this
	// session — display-only, kept separate from Status (see sessionRec.AckMs
	// and SelectSession). Additive field: append only, never reorder/rename
	// the fields above — this struct's JSON shape is a locked cross-plan
	// contract.
	AckMs int64 `json:"ackMs"`
}

// CompanionState is the whole room: every session, which one is selected, and
// the finish counter the speech bubble reads.
type CompanionState struct {
	Sessions     []SessionMood `json:"sessions"`
	Selected     string        `json:"selected"`
	Running      int           `json:"running"`
	Finished     int           `json:"finished"`
	FinishSeq    int           `json:"finishSeq"`
	LastFinished string        `json:"lastFinished"`
	// FreshInstall is true for the whole process's life iff config.toml did
	// not exist when startup() ran (see detectFreshInstall in app.go) — feeds
	// the freshInstall ambient condition (pack.js CONDITION_CLASS).
	FreshInstall bool `json:"freshInstall"`
}

// companionSnapshot derives the per-session state from the live registry.
// Sessions are sorted by window id so the room's hit targets keep a stable
// order and never move under the pointer because something finished elsewhere
// (§4.7). The sort is lexicographic, so "@10" precedes "@2" — stability is the
// property that matters, not numeric order.
func (a *App) companionSnapshot() CompanionState {
	companionSnapshotCallsForTest.Add(1)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.testCompanion != nil {
		return *a.testCompanion
	}
	st := CompanionState{
		Selected: a.current, FinishSeq: a.finishSeq, LastFinished: a.lastFinished,
		FreshInstall: a.freshInstall,
	}
	ids := make([]string, 0, len(a.sessions))
	for id := range a.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := a.sessions[id]
		st.Sessions = append(st.Sessions, SessionMood{
			WindowID: id, Name: r.Name, Status: r.Status,
			StatusSinceMs: r.StatusSinceMs, LastFinishMs: r.LastFinishMs, ErrorMs: r.ErrorMs,
			AckMs: r.AckMs,
		})
		switch r.Status {
		case "finished":
			st.Finished++
		case "active":
			st.Running++
		}
	}
	return st
}

// CompanionState is the Wails binding the deck polls as a fallback for the
// companion:state event (§6.2).
func (a *App) CompanionState() CompanionState { return a.companionSnapshot() }

// legacyOverlayState narrows a per-session snapshot to the flat shape the
// overlay polls. QuotaHigh stays false: PlanUsage is a live HTTPS call
// that triggers a keychain prompt and is unavailable to API-key users, so
// quota is deferred (§6.4).
//
// Finished/Awaiting are NOT a raw count of Status=="finished" sessions: the
// Stop hook fires on every assistant turn, so a session's Status stays
// "finished" for the rest of the process's life, and SelectSession must
// never rewrite Status (see its doc comment). Instead a session only counts
// here while it is UNACKNOWLEDGED since its last finish — AckMs (set by
// SelectSession) older than LastFinishMs. That makes selecting a finished
// session clear Awaiting without touching Status, and makes the ack a
// one-shot dismissal rather than a permanent mute: a session that finishes
// AGAIN after being acknowledged bumps LastFinishMs back past AckMs and
// counts once more.
//
// Takes the CompanionState rather than snapshotting itself, so a caller that
// needs both the flat and the per-session shape (pushCompanion) derives them
// from ONE snapshot instead of two — two snapshots would let a finish land
// between them and feed the overlay and the deck different states for what
// is logically a single push.
func (a *App) legacyOverlayState(st CompanionState) companionState {
	out := companionState{
		Running:   st.Running,
		FinishSeq: st.FinishSeq, LastFinished: st.LastFinished,
	}
	for _, s := range st.Sessions {
		if s.ErrorMs != 0 {
			out.Error = true
		}
		if s.Status == "finished" && s.AckMs < s.LastFinishMs {
			out.Finished++
		}
	}
	out.Awaiting = out.Finished > 0
	return out
}

// companionStateJSON is the overlay's poll payload.
func (a *App) companionStateJSON() string {
	b, _ := json.Marshal(a.legacyOverlayState(a.companionSnapshot()))
	return string(b)
}

// pushCompanion feeds both companion hosts from ONE snapshot: the desktop
// overlay by its loopback poll (no-op off darwin) and the deck by event. A
// single snapshot matters here, not just as an optimisation — two separate
// companionSnapshot() calls (one via companionStateJSON, one for the event)
// each take a.mu independently, so a finish landing between them would feed
// the overlay and the deck two DIFFERENT states for what the caller intended
// as a single, atomic push. Callers must NOT hold a.mu (companionSnapshot
// takes it).
func (a *App) pushCompanion() {
	st := a.companionSnapshot()
	b, _ := json.Marshal(a.legacyOverlayState(st))
	overlayPushFn(string(b))
	if a.emitter != nil {
		a.emitter.Emit("companion:state", st)
	}
}

// setCompanionCountsForTest is the deterministic test seam: it short-circuits
// companionSnapshot with a synthesized room so unit tests don't depend on live
// tmux/session state. awaiting is derived (Finished > 0) and kept in the
// signature only for the existing callers.
func (a *App) setCompanionCountsForTest(running, finished int, awaiting, errored bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	nowMs := a.now().UnixMilli()
	st := &CompanionState{Running: running, Finished: finished}
	for i := 0; i < running; i++ {
		st.Sessions = append(st.Sessions, SessionMood{
			WindowID: fmt.Sprintf("@r%d", i), Status: "active", StatusSinceMs: nowMs})
	}
	for i := 0; i < finished; i++ {
		st.Sessions = append(st.Sessions, SessionMood{
			WindowID: fmt.Sprintf("@f%d", i), Status: "finished", StatusSinceMs: nowMs, LastFinishMs: nowMs})
	}
	if errored && len(st.Sessions) > 0 {
		st.Sessions[0].ErrorMs = nowMs
	}
	a.testCompanion = st
}

// CompanionConfig is the Go-owned, persisted desktop-companion configuration:
// whether the overlay shows at all, its size/opacity, per-region jiggle
// intensity, and an optional user-picked model path.
// CompanionJiggle holds the per-group spring-stiffness multipliers (named so
// Wails can generate a clean TS type — an anonymous nested struct can't be).
type CompanionJiggle struct {
	Hair  float64 `json:"hair"`
	Bust  float64 `json:"bust"`
	Skirt float64 `json:"skirt"`
}

// avatarKindAvailable is only a meaningful signal on darwin. It is false by
// default and set true by overlay_wire.go's init(), which carries no build
// tag — so on a PRIVATE non-darwin build (Windows, Linux) it is also true,
// even though overlay_other.go (`//go:build !darwin`) stubs every overlay
// call to a no-op there. Such a build accepts kind=avatar and renders
// nothing: the flag alone does not prove the overlay works, only that
// overlay_wire.go was compiled in.
//
// On the PUBLIC export, scripts/export-public.sh deletes overlay*.go
// entirely, so the flag stays false and clamped() demotes the avatar kind —
// that half of the story is still accurate.
//
// This is a known, deliberately-deferred gap that predates this branch, not
// something introduced here. The obvious fix — tag overlay_wire.go
// `//go:build darwin` — is unsafe: the private frontend imports five overlay
// bindings by name (see the seam vars below), and `wails generate` omits
// build-tagged-out bindings on a Windows build, breaking the frontend build
// rather than just no-oping at runtime.
var avatarKindAvailable = false

// Companion kinds are mutually exclusive (§2.1). Exclusivity is enforced in Go
// at overlay-creation time, not in the frontend: a frontend check would allow
// two pollers to overlap during a transition.
const (
	KindAvatar = "avatar" // 3D avatar in the native desktop overlay window
	KindPanel  = "panel"  // in-deck holo-room (Plan 3)
	KindOff    = "off"
)

// companionHostOS is runtime.GOOS behind a variable so the darwin-only
// migration is testable on every runner.
var companionHostOS = runtime.GOOS

type CompanionConfig struct {
	// Enabled is the pre-Kind flag. It is an INPUT exactly once, during
	// migration; after that clamped() rewrites it as a read-only mirror of
	// Kind == KindAvatar so an older build reading this file still behaves.
	Enabled   bool            `json:"enabled"`
	Kind      string          `json:"kind"`
	Size      int             `json:"size"`
	Opacity   float64         `json:"opacity"`
	Jiggle    CompanionJiggle `json:"jiggle"`
	ModelPath string          `json:"modelPath"` // avatar kind
	PackPath  string          `json:"packPath"`  // panel kind: the pack FOLDER
}

// defaultCompanionConfig is what a fresh install (or a corrupt/missing
// companion.json) falls back to. New installs default to KindOff on EVERY
// platform: defaulting non-darwin to a panel would put an unrequested figure
// behind every Windows user's terminal on first run (§6.1).
func defaultCompanionConfig() CompanionConfig {
	c := CompanionConfig{Kind: KindOff, Size: 340, Opacity: 1.0}
	c.Jiggle.Hair, c.Jiggle.Bust, c.Jiggle.Skirt = 1, 1, 1
	return c
}

// migrateCompanionKind derives Kind for a config written before Kind existed.
// The overlay only ever existed on darwin, so that is the only case that
// becomes an avatar; everything else preserves "no companion".
func migrateCompanionKind(c CompanionConfig) CompanionConfig {
	if c.Enabled && companionHostOS == "darwin" {
		c.Kind = KindAvatar
	} else {
		c.Kind = KindOff
	}
	return c
}

// companionConfigPath sits beside config.toml; configPath() already honors
// COMMANDER_CONFIG, so redirecting that in tests redirects this too.
func companionConfigPath() string {
	return filepath.Join(filepath.Dir(configPath()), "companion.json")
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clamped returns c with every user-tunable field clamped to its valid range,
// an unknown Kind snapped to the platform default, and a PackPath that no
// longer resolves to a directory cleared (§6.1).
func (c CompanionConfig) clamped() CompanionConfig {
	switch c.Kind {
	case KindAvatar:
		if !avatarKindAvailable {
			c.Kind = KindOff
		}
	case KindPanel, KindOff:
	default:
		c.Kind = KindOff
	}
	if c.PackPath != "" {
		if fi, err := os.Stat(c.PackPath); err != nil || !fi.IsDir() {
			c.PackPath = ""
		}
	}
	c.Enabled = c.Kind == KindAvatar // read-only legacy mirror
	c.Size = clampI(c.Size, 160, 520)
	c.Opacity = clampF(c.Opacity, 0.2, 1.0)
	c.Jiggle.Hair = clampF(c.Jiggle.Hair, 0, 3)
	c.Jiggle.Bust = clampF(c.Jiggle.Bust, 0, 3)
	c.Jiggle.Skirt = clampF(c.Jiggle.Skirt, 0, 3)
	return c
}

// loadCompanionConfig reads companion.json, falling back to defaults if it's
// missing or corrupt. Unmarshaling onto a defaults value means a partial file
// (or one from an older schema) keeps defaults for any field it omits. Kind is
// blanked before unmarshalling and migrated after — the blank is the sentinel
// that distinguishes "file written before Kind existed" from "file that says
// off".
func loadCompanionConfig() CompanionConfig {
	b, err := os.ReadFile(companionConfigPath())
	if err != nil {
		return defaultCompanionConfig()
	}
	c := defaultCompanionConfig()
	c.Kind = "" // sentinel: a file with no "kind" key is pre-Kind -> migrate
	if json.Unmarshal(b, &c) != nil {
		return defaultCompanionConfig()
	}
	if c.Kind == "" {
		c = migrateCompanionKind(c)
	}
	return c.clamped()
}

// saveCompanionConfig persists c atomically (write to a temp file, then
// rename) so a crash mid-write can never leave a half-written companion.json.
func saveCompanionConfig(c CompanionConfig) error {
	p := companionConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c.clamped(), "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Overlay entry points behind variables, defaulting to no-ops. overlay_wire.go
// reassigns them in init(); the public export deletes that file, so a build
// without the overlay calls nothing. Two things this buys: app.go compiles
// identically in both trees (no override), and "kind X never creates the
// window" stays provable on every platform.
//
// The no-op default alone would fail SILENTLY, which is why clamped() demotes
// the avatar kind when avatarKindAvailable is false — on the PUBLIC export,
// where overlay*.go (including overlay_wire.go) is deleted entirely. On a
// PRIVATE non-darwin build, overlay_wire.go still compiles (no build tag) and
// sets avatarKindAvailable true, while overlay_other.go's `//go:build
// !darwin` stubs these same vars' callees to no-ops — so clamped() does NOT
// demote the kind there, and a Windows/Linux private build silently accepts
// kind=avatar and shows nothing. This is a known, deliberately-deferred gap
// predating this branch: tagging overlay_wire.go to darwin-only would be the
// obvious fix, but the private frontend imports these five overlay bindings
// by name, and `wails generate` omits a build-tagged-out file's bindings on
// Windows, so tagging it breaks the frontend build instead. Do not add a seam
// here without asking what a build that no-ops it shows the user.
var (
	overlayCloseFn = func() {}
	overlayPushFn  = func(json string) {}
	// applyAvatarKindFn raises the native overlay window for the avatar kind.
	applyAvatarKindFn = func(a *App, c CompanionConfig) {}
	// overlayRoutesFn registers the overlay page's loopback routes on the
	// local mux. Called from startWS.
	overlayRoutesFn = func(a *App, mux *http.ServeMux) {}
)

// applyCompanionKind brings the desktop overlay in line with the configured
// Kind. It is the ONLY place the overlay window is created, which is what
// makes the kinds mutually exclusive (§6.1). Idempotent: safe to call after
// any config change.
func (a *App) applyCompanionKind() {
	c := a.GetCompanionConfig()
	if c.Kind != KindAvatar {
		overlayCloseFn()
		return
	}
	applyAvatarKindFn(a, c)
}

// initCompanion is the startup hook: load the persisted config, reconcile the
// overlay with its Kind, and push the first state snapshot.
func (a *App) initCompanion() {
	a.companionMu.Lock()
	a.companion = loadCompanionConfig()
	a.companionMu.Unlock()
	a.applyCompanionKind()
	a.pushCompanion()
}

// GetCompanionKind returns the configured kind ("avatar" | "panel" | "off").
// Normalizes like clamped() does: a zero-value/unknown Kind (e.g. an App
// whose companion config was never loaded via initCompanion/loadCompanionConfig)
// reads as KindOff rather than "", so a rejected SetCompanionKind never leaves
// GetCompanionKind reporting a state that isn't one of the three valid kinds.
func (a *App) GetCompanionKind() string {
	switch k := a.GetCompanionConfig().Kind; k {
	case KindAvatar, KindPanel, KindOff:
		return k
	default:
		return KindOff
	}
}

// SetCompanionKind persists the kind and reconciles the overlay. An unknown
// kind is rejected outright rather than silently snapped, so a typo from the
// frontend surfaces instead of quietly turning the companion off.
func (a *App) SetCompanionKind(kind string) error {
	switch kind {
	case KindAvatar:
		if !avatarKindAvailable {
			return fmt.Errorf("this build has no desktop overlay")
		}
	case KindPanel, KindOff:
	default:
		return fmt.Errorf("unknown companion kind %q", kind)
	}
	if err := a.saveAndSyncCompanion(func(c *CompanionConfig) { c.Kind = kind }); err != nil {
		return err
	}
	a.applyCompanionKind()
	return nil
}
