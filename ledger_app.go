package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/codecity"
	"github.com/halalgami/CodingAgentCommander/internal/gitstamp"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/ingest"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scheduler"
	"github.com/halalgami/CodingAgentCommander/internal/progress"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
	"github.com/halalgami/CodingAgentCommander/internal/safegit"
	"github.com/halalgami/CodingAgentCommander/internal/transcripts"
)

// newSessionID returns an RFC 4122 v4 UUID for `claude --session-id`, so
// Commander knows a session's id from launch instead of guessing it by cwd.
func newSessionID() string {
	var b [16]byte
	_, _ = crand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// resumeID returns the id after --resume in args, or "".
func resumeID(args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--resume" {
			return args[i+1]
		}
	}
	return ""
}

// heroSig is what a live card shows from a hero: a change in any part re-emits dash:hero.
type heroSig struct {
	level, tier int
	loot        progress.Loot
	mixBucket   int64 // tool calls ÷ 10, so the radar refreshes without an event per call
}

func sigOf(h progress.Hero) heroSig {
	var n int64
	for _, v := range h.Mix {
		n += v
	}
	return heroSig{h.Level, h.Tier, h.Loot, n / 10}
}

// ledgerSvc owns the live tracker, the ingester and (after opt-in) the store.
type ledgerSvc struct {
	app          *App
	projectsRoot string
	tracker      *live.Tracker
	ing          *ingest.Ingester
	focused      atomic.Bool
	consentMu    sync.Mutex // serializes opt-in/out so two toggles can't each open a store

	mu       sync.Mutex
	store    *store.Store
	progress ingest.Progress
	cancelBF context.CancelFunc
	ctx      context.Context

	pendMu   sync.Mutex
	pendSess map[string]live.Session
	pendProj map[string]bool
	pendEv   []live.TimelineEvent
	pendGone []string

	resolver *repoid.Resolver
	sched    *scheduler.Scheduler
	mmu      sync.Mutex
	flagged  map[string]map[string]bool // repo root → flagged rel paths (latest scan)
	latest   map[string]scan.Result     // repo root → latest scan, memory only

	backfilling atomic.Bool   // achievement evaluation stays silent while true
	bfGen       atomic.Uint64 // the current backfill; a stale one may not end it
	heroMu      sync.Mutex
	heroes      map[string]heroSig // sessionID → last emitted signature
	// evalMu serialises evaluateKey: the ingester and the metrics scan both
	// evaluate, and a hero computed from older data must not be emitted last.
	evalMu sync.Mutex
}

func newLedgerSvc(a *App) *ledgerSvc {
	l := &ledgerSvc{app: a, projectsRoot: a.projectsRoot(), pendSess: map[string]live.Session{}, pendProj: map[string]bool{}, ctx: context.Background()}
	l.tracker = live.New(l.queueSession)
	l.tracker.OnEvent = l.queueEvents
	l.tracker.OnRemove = l.queueGone
	l.resolver = repoid.NewResolver()
	l.ing = ingest.New(l.projectsRoot, l.tracker, l.resolver)
	l.sched = scheduler.New(time.Now)
	l.flagged = map[string]map[string]bool{}
	l.latest = map[string]scan.Result{}
	l.heroes = map[string]heroSig{}
	l.ing.OnEdit = func(cwd string) {
		if root := l.resolver.Toplevel(context.Background(), cwd); root != "" {
			l.sched.Touch(root)
		}
	}
	l.ing.OnProgress = l.onProgress
	l.ing.OnProject = l.queueProject
	return l
}

func (a *App) ledgerDir() string { return filepath.Join(filepath.Dir(configPath()), "ledger") }

// start runs at app startup: reopen the store if the user opted in earlier,
// catch up, and start the read loop and the event pump.
func (l *ledgerSvc) start(ctx context.Context) {
	l.ctx = ctx
	l.ing.Root = l.projectsRoot
	if store.Enabled(l.app.ledgerDir()) {
		if st, err := store.Open(l.app.ledgerDir(), time.Now); err == nil {
			l.attach(st)
		} else {
			l.app.reportError("project history: " + err.Error())
		}
	}
	go l.ing.Run(ctx, l.focused.Load)
	go l.pump(ctx)
	go l.sched.Run(ctx, l.scanRepo, l.onMetrics)
	go l.metricsLoop(ctx)
}

func (l *ledgerSvc) attach(st *store.Store) {
	l.mu.Lock()
	l.store = st
	bctx, cancel := context.WithCancel(l.ctx)
	l.cancelBF = cancel
	l.mu.Unlock()
	l.ing.Root = l.projectsRoot
	l.ing.SetStore(st)
	gen := l.beginBackfill()
	go func() {
		err := l.ing.Backfill(bctx)
		l.endBackfill(gen)
		if err == nil {
			l.finishBackfill()
		}
		if err != nil {
			return
		}
		// First metrics pass: every existing project not scanned yet.
		for _, p := range st.Projects() {
			if !p.Missing && p.ScannedAt == 0 {
				l.sched.Queue(p.Root)
			}
		}
	}()
}

// beginBackfill marks evaluation silent and returns this backfill's generation.
func (l *ledgerSvc) beginBackfill() uint64 {
	gen := l.bfGen.Add(1)
	l.backfilling.Store(true)
	return gen
}

// endBackfill ends the silence only if gen is still the current backfill, so a
// cancelled one finishing after a re-attach cannot unsilence the new one.
func (l *ledgerSvc) endBackfill(gen uint64) {
	if l.bfGen.Load() == gen {
		l.backfilling.Store(false)
	}
}

func (l *ledgerSvc) detach() *store.Store {
	l.mu.Lock()
	st := l.store
	l.store = nil
	if l.cancelBF != nil {
		l.cancelBF()
		l.cancelBF = nil
	}
	l.progress = ingest.Progress{}
	l.mu.Unlock()
	l.ing.SetStore(nil)
	return st
}

func (l *ledgerSvc) stop() {
	if st := l.detach(); st != nil {
		_ = st.Close()
	}
}

func (l *ledgerSvc) currentStore() *store.Store {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.store
}

func (l *ledgerSvc) transcriptPath(cwd, sid string) string {
	return filepath.Join(l.projectsRoot, transcripts.EncodeCwd(cwd), sid+".jsonl")
}

func (l *ledgerSvc) watch(cwd, sid string)   { l.ing.Watch(l.transcriptPath(cwd, sid)) }
func (l *ledgerSvc) unwatch(cwd, sid string) { l.ing.Unwatch(l.transcriptPath(cwd, sid)) }

func (l *ledgerSvc) hook(sid, kind string) {
	k := live.HookKind(kind)
	switch k {
	case live.HookStop, live.HookPermission, live.HookIdle, live.HookNeedsInput:
		l.tracker.Hook(sid, k, time.Now().UnixMilli())
	}
}

func (l *ledgerSvc) onProgress(p ingest.Progress) {
	l.mu.Lock()
	l.progress = p
	l.mu.Unlock()
	if p.Done || p.Files%50 == 0 {
		l.app.emitter.Emit("ledger:progress", p)
	}
}

func (l *ledgerSvc) queueSession(s live.Session) {
	l.pendMu.Lock()
	l.pendSess[s.ID] = s
	l.pendMu.Unlock()
}

func (l *ledgerSvc) queueProject(key string) {
	l.pendMu.Lock()
	l.pendProj[key] = true
	l.pendMu.Unlock()
	l.evaluate(key)
}

// pump coalesces changes every 250 ms: dash:session per changed session, one
// dash:tick with every change and new timeline mark, dash:project per project
// every 2 s; the crew clock runs every second.
func (l *ledgerSvc) pump(ctx context.Context) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		n++
		l.flushPending(n)
	}
}

func (l *ledgerSvc) flushPending(n int) {
	if n%4 == 0 {
		l.tracker.Tick(time.Now().UnixMilli())
	}
	l.pendMu.Lock()
	sess := l.pendSess
	l.pendSess = map[string]live.Session{}
	evs := l.pendEv
	l.pendEv = nil
	gone := l.pendGone
	l.pendGone = nil
	var proj map[string]bool
	if n%8 == 0 {
		proj = l.pendProj
		l.pendProj = map[string]bool{}
	}
	l.pendMu.Unlock()
	tickSess := make([]live.Session, 0, len(sess))
	roots := l.roots()
	for _, s := range sess {
		s = relTargets(s, roots)
		l.app.emitter.Emit("dash:session", s)
		tickSess = append(tickSess, s)
	}
	if len(tickSess) > 0 || len(evs) > 0 || len(gone) > 0 {
		l.app.emitter.Emit("dash:tick", DashTick{Sessions: tickSess, Events: evs, Removed: gone})
	}
	if st := l.currentStore(); st != nil && len(proj) > 0 {
		for _, p := range st.Projects() {
			if proj[p.Key] {
				l.app.emitter.Emit("dash:project", p)
			}
		}
		l.app.emitter.Emit("dash:city")
	}
}

// DashTick is one batched dashboard update: changed sessions and new timeline marks.
type DashTick struct {
	Sessions []live.Session       `json:"sessions"`
	Events   []live.TimelineEvent `json:"events"`
	Removed  []string             `json:"removed"`
}

const maxPendingEvents = 2000

func (l *ledgerSvc) queueGone(ids []string) {
	l.pendMu.Lock()
	l.pendGone = append(l.pendGone, ids...)
	l.pendMu.Unlock()
	l.heroMu.Lock()
	for _, id := range ids {
		delete(l.heroes, id)
	}
	l.heroMu.Unlock()
}

func (l *ledgerSvc) queueEvents(evs []live.TimelineEvent) {
	l.pendMu.Lock()
	l.pendEv = append(l.pendEv, evs...)
	if len(l.pendEv) > maxPendingEvents {
		l.pendEv = l.pendEv[len(l.pendEv)-maxPendingEvents:]
	}
	l.pendMu.Unlock()
}

func (l *ledgerSvc) scanRepo(ctx context.Context, root string) (scan.Result, error) {
	return scan.Repo(ctx, root, scan.DefaultLimits, time.Now)
}

// onMetrics records a finished scan: in memory always (hotspot flag), on disk
// only while history is on.
func (l *ledgerSvc) onMetrics(root string, r scan.Result, err error) {
	if err != nil {
		return
	}
	// Key by the working tree's own top-level (symlinks resolved, a worktree
	// stays itself) — the same key isHotspot derives from a session's cwd.
	if top := l.resolver.Toplevel(context.Background(), root); top != "" {
		root = top
	}
	set := map[string]bool{}
	type hs struct {
		Path  string  `json:"path"`
		Score float64 `json:"score"`
	}
	var top []hs
	for p, f := range r.Files {
		if f.Flagged {
			set[p] = true
		}
		if f.Hotspot > 0 {
			top = append(top, hs{p, f.Hotspot})
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].Score > top[j].Score })
	if len(top) > 10 {
		top = top[:10]
	}
	l.mmu.Lock()
	l.flagged[root] = set
	l.latest[root] = r
	l.mmu.Unlock()
	if st := l.currentStore(); st != nil {
		if st.ApplyMetrics(root, r) {
			if id, ok := st.KeyByRoot(root); ok {
				l.evaluate(id.Key)
			}
		}
	}
	l.app.emitter.Emit("dash:metrics", map[string]any{"root": root, "snapshot": r.Snapshot, "hotspots": top})
}

// isHotspot reports whether targetPath is a flagged file of the repo cwd belongs to.
func (l *ledgerSvc) isHotspot(cwd, targetPath string) bool {
	if targetPath == "" || cwd == "" {
		return false
	}
	root := l.resolver.Toplevel(context.Background(), cwd)
	if root == "" {
		return false
	}
	// Resolve the directory, not the file: the file may not exist yet (a
	// Write about to create it), and macOS temp/home paths are symlinked.
	real := targetPath
	if d, err := filepath.EvalSymlinks(filepath.Dir(targetPath)); err == nil {
		real = filepath.Join(d, filepath.Base(targetPath))
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	l.mmu.Lock()
	defer l.mmu.Unlock()
	return l.flagged[root][filepath.ToSlash(rel)]
}

// metricsLoop: every minute, a HEAD check for projects with live sessions;
// every hour, the weekly rule for quiet projects. Both stat the HEAD reflog
// (gitstamp) rather than spawn git, which cost ~60 processes an hour per live
// repo; git runs only where there is no reflog to read.
func (l *ledgerSvc) metricsLoop(ctx context.Context) {
	heads := map[string]string{}
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		n++
		for _, s := range l.tracker.Snapshot() {
			if s.Cwd == "" {
				continue
			}
			root := l.resolver.Toplevel(ctx, s.Cwd)
			if root == "" {
				continue
			}
			head, ok := gitstamp.Stamp(root)
			if !ok {
				h, err := safegit.Run(ctx, root, "rev-parse", "HEAD")
				if err != nil {
					continue
				}
				head = h
			}
			if old, seen := heads[root]; !seen || old != head {
				heads[root] = head
				l.sched.HeadMoved(root)
			}
		}
		if n%60 != 0 {
			continue
		}
		st := l.currentStore()
		if st == nil {
			continue
		}
		for _, p := range st.Projects() {
			if p.Missing || time.Since(time.UnixMilli(p.ScannedAt)) < 7*24*time.Hour {
				continue
			}
			if moved, ok := gitstamp.Moved(p.Root); ok {
				if moved.UnixMilli() > p.ScannedAt {
					l.sched.Queue(p.Root)
				}
				continue
			}
			if head, err := safegit.Run(ctx, p.Root, "rev-parse", "HEAD"); err == nil && head != p.ScanHead {
				l.sched.Queue(p.Root)
			}
		}
	}
}

func (l *ledgerSvc) difficulty(st *store.Store, key string) float64 {
	for _, p := range st.Projects() {
		if p.Key == key && p.Latest != nil {
			return progress.Difficulty(p.Latest.Complexity, true)
		}
	}
	return 1
}

// evaluate runs achievement rules and hero levels for one project after it
// changed. Unlocks are recorded always; events only outside backfill.
func (l *ledgerSvc) evaluate(key string) {
	if l.backfilling.Load() {
		return // deferred: finishBackfill does one pass when the backfill ends
	}
	l.evaluateKey(key, false)
}

// finishBackfill evaluates every project once, silently, after a backfill.
func (l *ledgerSvc) finishBackfill() {
	st := l.currentStore()
	if st == nil {
		return
	}
	for _, p := range st.Projects() {
		l.evaluateKey(p.Key, true)
	}
}

func (l *ledgerSvc) evaluateKey(key string, silent bool) {
	l.evalMu.Lock()
	defer l.evalMu.Unlock()
	st := l.currentStore()
	if st == nil {
		return
	}
	p, ok := st.Progressable(key)
	if !ok {
		return
	}
	now := time.Now().UnixMilli()
	for _, u := range st.RecordUnlocks(key, progress.Evaluate(p, now)) {
		if !silent {
			l.app.emitter.Emit("dash:achievement", map[string]any{"id": u.ID, "name": progress.NameOf(u.ID), "projectKey": key, "sessionID": u.SessionID, "at": u.At})
		}
	}
	d := l.difficulty(st, key)
	for _, s := range p.Sessions {
		if _, live := l.tracker.Get(s.ID); !live {
			continue
		}
		h := progress.HeroOf(s, d)
		sig := sigOf(h)
		l.heroMu.Lock()
		prev, seen := l.heroes[s.ID]
		changed := !seen || prev != sig
		l.heroes[s.ID] = sig
		l.heroMu.Unlock()
		if changed && !silent {
			l.app.emitter.Emit("dash:hero", h)
		}
	}
	view := l.progressView()
	for _, u := range st.RecordAccountUnlocks(progress.EvaluateAccount(view.Commander.Level, haveIDs(st.AccountUnlocks()), now)) {
		if !silent {
			l.app.emitter.Emit("dash:achievement", map[string]any{"id": u.ID, "name": progress.NameOf(u.ID), "at": u.At})
		}
	}
}

func haveIDs(us []store.Unlock) map[string]bool {
	m := map[string]bool{}
	for _, u := range us {
		m[u.ID] = true
	}
	return m
}

func (l *ledgerSvc) progressView() ProgressView {
	v := ProgressView{Projects: []ProjectProgress{}, Live: []progress.Hero{}}
	st := l.currentStore()
	if st == nil {
		v.Locked = true
		return v
	}
	var total float64
	for _, info := range st.Projects() {
		d := 1.0
		if info.Latest != nil {
			d = progress.Difficulty(info.Latest.Complexity, true)
		}
		pp := ProjectProgress{Key: info.Key, Label: info.Label, Hours: float64(info.Totals.ActiveMs) / 3.6e6, Difficulty: d, Heroes: []progress.Hero{}}
		pp.Rank = progress.ProjectRank(pp.Hours)
		// Sessions + unlocks only: Project() would deep-copy per-file metrics.
		pp.Unlocks = st.Unlocks(info.Key)
		{
			for _, s := range st.Sessions(info.Key) {
				h := progress.HeroOf(s, d)
				total += h.XP
				pp.Heroes = append(pp.Heroes, h)
				if _, live := l.tracker.Get(s.ID); live {
					v.Live = append(v.Live, h)
				}
			}
		}
		v.Projects = append(v.Projects, pp)
	}
	lv := progress.CommanderLevel(total)
	v.Commander = CommanderView{Level: lv, XP: total, Next: 50 * float64((lv+1)*(lv+1)), Unlocks: st.AccountUnlocks()}
	return v
}

// providers resolves the catalog once and maps a transcript model id to its
// provider ("" if the catalog does not list it).
func (l *ledgerSvc) providers() func(string) string {
	m := map[string]string{}
	for _, x := range l.app.snapshotModels() {
		m[x.ID] = x.Provider
	}
	return func(model string) string { return m[model] }
}

// relTargets fills TargetRel (lead and crew) relative to the deepest project
// root containing the session's cwd; a file outside that root gets "". Only
// these relative paths reach the frontend (CodeCity cranes, phase 5b).
func relTargets(s live.Session, roots map[string]string) live.Session {
	root := deepestRoot(s.Cwd, roots)
	rel := func(p string) string {
		if root == "" || p == "" {
			return ""
		}
		r, err := filepath.Rel(root, p)
		if err != nil {
			return ""
		}
		r = filepath.ToSlash(r)
		if !codecity.Valid(r) {
			return ""
		}
		return r
	}
	s.TargetRel = rel(s.TargetPath)
	if len(s.Crew) > 0 {
		crew := make([]live.Agent, len(s.Crew))
		copy(crew, s.Crew)
		for i := range crew {
			crew[i].TargetRel = rel(crew[i].TargetPath)
		}
		s.Crew = crew
	}
	return s
}

func (l *ledgerSvc) roots() map[string]string {
	if st := l.currentStore(); st != nil {
		return st.Roots()
	}
	return nil
}

// deepestRoot is the longest project root containing cwd ("" if none).
func deepestRoot(cwd string, roots map[string]string) string {
	root := ""
	for _, r := range roots {
		if r != "" && (cwd == r || strings.HasPrefix(cwd, r+string(filepath.Separator))) && len(r) > len(root) {
			root = r
		}
	}
	return root
}

// liveTargets lists the relative files that live sessions (and their agents)
// whose deepest project root is root are working on — a session in a nested
// project never adds files to the parent's CodeView.
func liveTargets(sessions []live.Session, roots map[string]string, root string) []string {
	var out []string
	for _, s := range sessions {
		if root == "" || deepestRoot(s.Cwd, roots) != root {
			continue
		}
		s = relTargets(s, roots)
		if s.TargetRel != "" {
			out = append(out, s.TargetRel)
		}
		for _, ag := range s.Crew {
			if ag.TargetRel != "" {
				out = append(out, ag.TargetRel)
			}
		}
	}
	return out
}
