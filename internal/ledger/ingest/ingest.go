// Package ingest is the single owner of transcript reads. Each read feeds the
// live tracker and, after opt-in, the store; both keep their own position so a
// late opt-in back-fills the store without replaying events into the tracker.
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

var ErrNoStore = errors.New("ingest: project history is off")

type Progress struct {
	Files int   `json:"files"`
	Total int   `json:"total"`
	Bytes int64 `json:"bytes"`
	Done  bool  `json:"done"`
}

type Ingester struct {
	Root       string
	LiveWindow time.Duration
	Now        func() time.Time
	OnProgress func(Progress)
	OnProject  func(key string)
	// OnEdit is called with the cwd of every live edit-like tool call
	// (Edit/Write/MultiEdit/NotebookEdit/Bash): the metrics scheduler's trigger.
	OnEdit func(cwd string)

	tracker  *live.Tracker
	resolver *repoid.Resolver

	readMu  sync.Mutex // serializes ReadFile: backfill and the run loop share files
	mu      sync.Mutex
	store   *store.Store
	liveCur map[string]int64
	// skipped holds the size at which a transcript was read but not stored
	// (excluded root, or no cwd yet), so sweeps don't re-read it until it grows.
	skipped map[string]int64
	watched map[string]bool
}

func New(root string, t *live.Tracker, r *repoid.Resolver) *Ingester {
	return &Ingester{Root: root, LiveWindow: 2 * time.Minute, Now: time.Now, tracker: t, resolver: r,
		liveCur: map[string]int64{}, watched: map[string]bool{}, skipped: map[string]int64{}}
}

func (in *Ingester) SetStore(s *store.Store) {
	in.mu.Lock()
	in.store = s
	in.mu.Unlock()
}

func (in *Ingester) currentStore() *store.Store {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.store
}

func (in *Ingester) Watch(path string) {
	in.mu.Lock()
	in.watched[path] = true
	in.mu.Unlock()
}

func (in *Ingester) Unwatch(path string) {
	in.mu.Lock()
	delete(in.watched, path)
	in.mu.Unlock()
}

func (in *Ingester) isWatched(path string) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.watched[path]
}

// sessionOf derives the session from the path: <enc>/<id>.jsonl, or
// <enc>/<id>/subagents/<agent>.jsonl for subagent transcripts.
func sessionOf(path string) (id string, subagent bool) {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "subagents" {
		return filepath.Base(filepath.Dir(dir)), true
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl"), false
}

// ReadFile reads what was appended to path since the last read and dispatches
// it to the tracker (when toTracker) and the store (when attached).
func (in *Ingester) ReadFile(ctx context.Context, path string, toTracker bool) error {
	in.readMu.Lock()
	defer in.readMu.Unlock()

	st := in.currentStore()
	fi, err := os.Stat(path)
	if err != nil {
		if st != nil {
			st.DropCursor(path) // deleted by Claude Code cleanup: summary stays
		}
		return nil
	}
	size := fi.Size()
	sessionID, subagent := sessionOf(path)

	in.mu.Lock()
	lo, hasLive := in.liveCur[path]
	in.mu.Unlock()
	if !toTracker {
		lo = size
	} else if !hasLive || size < lo {
		if hasLive && !subagent { // a rewritten agent file must not forget its lead
			in.tracker.Forget(sessionID)
		}
		lo = 0
	}

	so := size
	var sc store.Cursor
	if st != nil {
		var ok bool
		sc, ok = st.Cursor(path)
		if ok && (size < sc.Size || sc.ParserVersion != parse.ParserVersion) {
			if key, found := in.projectKey(ctx, st, sc.Cwd); found && !subagent {
				st.ResetSession(key, sessionID)
			}
			sc = store.Cursor{Cwd: sc.Cwd}
		}
		sc.SessionID, sc.Subagent, sc.ParserVersion = sessionID, subagent, parse.ParserVersion
		so = sc.Offset
	}

	start := lo
	if so < start {
		start = so
	}
	if start >= size {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}

	pos := start
	cwd := sc.Cwd
	var liveEvs, storeEvs []parse.Event
	var skipped int64
	_, _, rerr := parse.ReadLines(f, func(line []byte, n int64) {
		lineStart := pos
		pos += n
		wantLive := toTracker && lineStart >= lo
		wantStore := st != nil && lineStart >= so
		if line == nil {
			if wantStore {
				skipped++
			}
			return
		}
		if !wantLive && !wantStore {
			return
		}
		evs := parse.Classify(line)
		for i := range evs {
			if evs[i].SessionID == "" || subagent {
				evs[i].SessionID = sessionID
			}
			if cwd == "" && evs[i].Cwd != "" {
				cwd = evs[i].Cwd
			}
		}
		if wantLive {
			liveEvs = append(liveEvs, evs...)
		}
		if wantStore {
			storeEvs = append(storeEvs, evs...)
		}
	})
	if rerr != nil {
		return rerr
	}

	if toTracker {
		if subagent {
			in.tracker.ApplyAgent(sessionID, agentOf(path), readMeta(path), liveEvs, !in.isWatched(leadOf(path)))
		} else {
			in.tracker.Apply(liveEvs, !in.isWatched(path))
		}
		if in.OnEdit != nil {
			for _, e := range liveEvs {
				if e.Kind != parse.ToolUse || !editTools[e.ToolName] {
					continue
				}
				c := e.Cwd
				if c == "" {
					c = cwd
				}
				if c != "" {
					in.OnEdit(c)
				}
			}
		}
		in.mu.Lock()
		in.liveCur[path] = pos
		in.mu.Unlock()
	}
	if st != nil && in.currentStore() == st {
		if cwd == "" {
			in.markSkipped(path, size) // cannot place it yet; retry when it grows
			return nil
		}
		id := in.resolve(ctx, st, cwd)
		sc.Offset, sc.Size, sc.Mtime, sc.Cwd = pos, size, fi.ModTime().UnixMilli(), cwd
		if !st.Apply(id, path, sc, subagent, storeEvs, skipped) {
			in.markSkipped(path, size) // excluded root
			return nil
		}
		in.mu.Lock()
		delete(in.skipped, path)
		in.mu.Unlock()
		if in.OnProject != nil {
			in.OnProject(id.Key)
		}
	}
	return nil
}

func (in *Ingester) projectKey(ctx context.Context, st *store.Store, cwd string) (string, bool) {
	if cwd == "" {
		return "", false
	}
	return in.resolve(ctx, st, cwd).Key, true
}

// resolve picks the project for a cwd: a deleted folder keeps its repo's
// history (KeyByRoot), a deleted folder inside a live repo folds into it, and
// a folder that became a repo migrates once (Merge).
func (in *Ingester) resolve(ctx context.Context, st *store.Store, cwd string) repoid.ID {
	id := in.resolver.Resolve(ctx, cwd)
	if id.Gone != "" {
		if known, ok := st.KeyByRoot(id.Gone); ok && !strings.HasPrefix(known.Identity, "path:") {
			return known // a deleted repo nested in another keeps its own history
		}
	}
	if strings.HasPrefix(id.Identity, "path:") {
		if known, ok := st.KeyByRoot(id.Root); ok {
			return known
		}
		return id
	}
	if id.PathKey != "" && id.PathKey != id.Key && st.Has(id.PathKey) {
		st.Merge(id.PathKey, id)
	}
	return id
}

type fileInfo struct {
	path  string
	size  int64
	mtime int64
}

func (in *Ingester) list() []fileInfo {
	var out []fileInfo
	_ = filepath.WalkDir(in.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "memory" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".jsonl" {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			out = append(out, fileInfo{p, fi.Size(), fi.ModTime().UnixMilli()})
		}
		return nil
	})
	return out
}

func (in *Ingester) recent(mtime int64) bool {
	return in.Now().Sub(time.UnixMilli(mtime)) < in.LiveWindow
}

// Backfill reads every transcript into the store, newest first. It yields
// between files and stops as soon as the store is detached or ctx ends.
func (in *Ingester) Backfill(ctx context.Context) error {
	files := in.list()
	sort.Slice(files, func(i, j int) bool { return files[i].mtime > files[j].mtime })
	p := Progress{Total: len(files)}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if in.currentStore() == nil {
			return ErrNoStore
		}
		toLive := in.recent(f.mtime) || in.isWatched(f.path)
		if _, sub := sessionOf(f.path); sub {
			toLive = in.Now().Sub(time.UnixMilli(f.mtime)) < AgentWindow
		}
		_ = in.ReadFile(ctx, f.path, toLive)
		p.Files++
		p.Bytes += f.size
		if in.OnProgress != nil {
			in.OnProgress(p)
		}
		runtime.Gosched()
	}
	p.Done = true
	if in.OnProgress != nil {
		in.OnProgress(p)
	}
	in.Flush()
	return nil
}

// Sweep catches work done outside Commander: a size check of every transcript,
// reading only what changed. Without a store only recent files are read, and
// only for the tracker.
func (in *Ingester) Sweep(ctx context.Context) error {
	st := in.currentStore()
	for _, f := range in.list() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rec := in.recent(f.mtime)
		if _, sub := sessionOf(f.path); sub {
			rec = in.Now().Sub(time.UnixMilli(f.mtime)) < AgentWindow
		}
		in.mu.Lock()
		lo, hasLive := in.liveCur[f.path]
		in.mu.Unlock()
		liveStale := rec && (!hasLive || lo != f.size)
		storeStale := st != nil && in.storeStale(st, f)
		if !liveStale && !storeStale {
			continue
		}
		_ = in.ReadFile(ctx, f.path, rec || in.isWatched(f.path))
	}
	in.tracker.Expire(in.Now().UnixMilli(), 15*time.Minute)
	return nil
}

// Tail reads every watched (Commander-launched) transcript.
func (in *Ingester) Tail(ctx context.Context) {
	in.mu.Lock()
	paths := make([]string, 0, len(in.watched))
	for p := range in.watched {
		paths = append(paths, p)
	}
	in.mu.Unlock()
	for _, p := range paths {
		_ = in.ReadFile(ctx, p, true)
		for _, a := range in.recentAgents(p) {
			_ = in.ReadFile(ctx, a, true)
		}
	}
}

func (in *Ingester) Flush() {
	if st := in.currentStore(); st != nil {
		_ = st.Flush()
	}
}

// Run tails watched sessions every 1 s when fast() (window focused) else 5 s,
// sweeps every minute, flushes every 5 s, and flushes on exit.
func (in *Ingester) Run(ctx context.Context, fast func() bool) {
	var lastSweep time.Time
	lastFlush := in.Now()
	for {
		d := 5 * time.Second
		if fast != nil && fast() {
			d = time.Second
		}
		select {
		case <-ctx.Done():
			in.Flush()
			return
		case <-time.After(d):
		}
		in.Tail(ctx)
		now := in.Now()
		if now.Sub(lastSweep) >= time.Minute {
			_ = in.Sweep(ctx)
			lastSweep = now
		}
		if now.Sub(lastFlush) >= 5*time.Second {
			in.Flush()
			lastFlush = now
		}
	}
}

// storeStale reports whether f has bytes the store has not taken yet.
func (in *Ingester) storeStale(st *store.Store, f fileInfo) bool {
	in.mu.Lock()
	sz, skip := in.skipped[f.path]
	in.mu.Unlock()
	if skip && sz == f.size {
		return false
	}
	c, ok := st.Cursor(f.path)
	return !ok || c.Size != f.size
}

func (in *Ingester) markSkipped(path string, size int64) {
	in.mu.Lock()
	in.skipped[path] = size
	in.mu.Unlock()
}

var editTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true, "Bash": true}

// AgentWindow is how recently a subagent file must have been written to be
// tailed live.
const AgentWindow = 10 * time.Minute

// agentOf returns the agent id from <...>/subagents/agent-<id>.jsonl.
func agentOf(path string) string {
	return strings.TrimPrefix(strings.TrimSuffix(filepath.Base(path), ".jsonl"), "agent-")
}

// leadOf returns the lead transcript path of a subagent transcript.
func leadOf(path string) string {
	sessionDir := filepath.Dir(filepath.Dir(path))
	return filepath.Join(filepath.Dir(sessionDir), filepath.Base(sessionDir)+".jsonl")
}

// readMeta reads agent-<id>.meta.json beside a subagent transcript: at most
// 4 KB of JSON. Anything else yields a zero meta (the tracker falls back).
func readMeta(path string) live.AgentMeta {
	f, err := os.Open(strings.TrimSuffix(path, ".jsonl") + ".meta.json")
	if err != nil {
		return live.AgentMeta{}
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return live.AgentMeta{}
	}
	var m live.AgentMeta
	if json.Unmarshal(b, &m) != nil {
		return live.AgentMeta{}
	}
	return m
}

// recentAgents lists a lead's subagent transcripts written within AgentWindow.
func (in *Ingester) recentAgents(lead string) []string {
	dir := filepath.Join(strings.TrimSuffix(lead, ".jsonl"), "subagents")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		if fi, err := e.Info(); err == nil && in.Now().Sub(fi.ModTime()) < AgentWindow {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}
