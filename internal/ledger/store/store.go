package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

var ErrNoConsent = errors.New("ledger: project history is off")

const consentFile = "consent.json"

type Consent struct {
	Enabled     bool     `json:"enabled"`
	DecidedAt   int64    `json:"decidedAt"`
	StoreTitles bool     `json:"storeTitles"`
	Excluded    []string `json:"excluded,omitempty"`
}

type ProjectInfo struct {
	Key       string         `json:"key"`
	Label     string         `json:"label"`
	Root      string         `json:"root"`
	Missing   bool           `json:"missing"`
	FirstSeen int64          `json:"firstSeen"`
	LastSeen  int64          `json:"lastSeen"`
	Totals    Counters       `json:"totals"`
	ScannedAt int64          `json:"scannedAt"`
	ScanHead  string         `json:"scanHead"`
	Latest    *scan.Snapshot `json:"latest"`
	Plot      int            `json:"plot"`
}

type Store struct {
	dir      string
	now      func() time.Time
	mu       sync.Mutex
	consent  Consent
	projects map[string]*Project
	byPath   map[string]string
	dirty    map[string]bool
	readOnly bool
	unlock   func()
	lastErr  string

	account      []Unlock // account-wide unlocks (profile.json)
	profileDirty bool
}

// Enabled reports whether the user opted in. It never creates anything.
func Enabled(dir string) bool {
	var c Consent
	return readJSON(filepath.Join(dir, consentFile), &c) == nil && c.Enabled
}

// Create is the opt-in: it makes the directory (0700), records consent, opens.
func Create(dir string, c Consent, now func() time.Time) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o700); err != nil {
		return nil, err
	}
	_ = os.Chmod(dir, 0o700)
	c.Enabled = true
	if c.DecidedAt == 0 {
		c.DecidedAt = now().UnixMilli()
	}
	if err := writeJSON(filepath.Join(dir, consentFile), c); err != nil {
		return nil, err
	}
	return Open(dir, now)
}

func Open(dir string, now func() time.Time) (*Store, error) {
	var c Consent
	if err := readJSON(filepath.Join(dir, consentFile), &c); err != nil || !c.Enabled {
		return nil, ErrNoConsent
	}
	unlock, ok, err := lockDir(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, now: now, consent: c, projects: map[string]*Project{}, byPath: map[string]string{},
		dirty: map[string]bool{}, readOnly: !ok, unlock: unlock}
	entries, _ := os.ReadDir(filepath.Join(dir, "projects"))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		s.load(strings.TrimSuffix(name, ".json"))
	}
	s.assignPlotsLocked()
	var prof struct {
		Unlocks []Unlock `json:"unlocks"`
	}
	profPath := filepath.Join(dir, "profile.json")
	if err := readJSON(profPath, &prof); err == nil {
		s.account = prof.Unlocks
	} else if !os.IsNotExist(err) {
		// Keep the unreadable file aside so the next flush cannot destroy it;
		// account achievements are re-earned from the level as it is evaluated.
		_ = os.Rename(profPath, fmt.Sprintf("%s.corrupt-%d", profPath, s.now().Unix()))
		s.lastErr = "account achievements were unreadable; the file was kept aside and they are re-earned as you play"
	}
	return s, nil
}

func (s *Store) projectPath(key string) string { return filepath.Join(s.dir, "projects", key+".json") }

func (s *Store) load(key string) {
	path := s.projectPath(key)
	var p Project
	err := readJSON(path, &p)
	if err != nil {
		_ = os.Rename(path, fmt.Sprintf("%s.corrupt-%d", path, s.now().Unix()))
		if berr := readJSON(path+".bak", &p); berr != nil {
			s.lastErr = fmt.Sprintf("project history for %s was unreadable and had no usable backup; it will be rebuilt from transcripts still on disk", key)
			return
		}
		s.lastErr = fmt.Sprintf("project history for %s was unreadable; restored from its daily backup — activity since then is re-read from transcripts still on disk", p.Label)
		s.dirty[key] = true
	}
	s.projects[key] = &p
	for path := range p.Cursors {
		s.byPath[path] = key
	}
}

func (s *Store) Consent() Consent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.consent
}

// SetConsent updates titles/exclusions (Enabled must stay true; opting out is
// DeleteAll or Close by the caller). Newly excluded roots are purged.
func (s *Store) SetConsent(c Consent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.Enabled, c.DecidedAt = true, s.consent.DecidedAt
	s.consent = c
	if !c.StoreTitles {
		for key, p := range s.projects {
			for i := range p.Sessions {
				if p.Sessions[i].Title != "" {
					p.Sessions[i].Title = ""
					s.dirty[key] = true
				}
			}
		}
	}
	for key, p := range s.projects {
		if s.isExcluded(p.Root) {
			s.forgetLocked(key)
		}
	}
	if s.readOnly {
		return nil
	}
	return writeJSON(filepath.Join(s.dir, consentFile), c)
}

func (s *Store) isExcluded(root string) bool {
	for _, ex := range s.consent.Excluded {
		ex = filepath.Clean(ex)
		if root == ex || strings.HasPrefix(root, ex+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Apply folds one read of one transcript into its project, together with the
// cursor that read reached. Returns false if the root is excluded.
func (s *Store) Apply(id repoid.ID, path string, cur Cursor, subagent bool, evs []parse.Event, skipped int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isExcluded(id.Root) {
		return false
	}
	p := s.projects[id.Key]
	if p == nil {
		p = &Project{Key: id.Key, Identity: id.Identity, Root: id.Root, Label: id.Label}
		s.projects[id.Key] = p
		p.Plot = s.nextPlotLocked()
	}
	sess := p.session(cur.SessionID)
	for _, e := range evs {
		p.fold(sess, e, subagent, s.consent.StoreTitles)
	}
	sess.Skipped += skipped
	if p.Cursors == nil {
		p.Cursors = map[string]Cursor{}
	}
	p.Cursors[path] = cur
	s.byPath[path] = id.Key
	p.recompute()
	s.dirty[id.Key] = true
	return true
}

func (s *Store) Cursor(path string) (Cursor, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byPath[path]
	if !ok {
		return Cursor{}, false
	}
	c, ok := s.projects[key].Cursors[path]
	return c, ok
}

// DropCursor forgets a transcript that no longer exists. Its summary stays.
func (s *Store) DropCursor(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key, ok := s.byPath[path]; ok {
		delete(s.projects[key].Cursors, path)
		delete(s.byPath, path)
		s.dirty[key] = true
	}
}

func (s *Store) ResetSession(key, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return
	}
	for path, c := range p.Cursors {
		if c.SessionID == sessionID {
			delete(s.byPath, path)
		}
	}
	p.resetSession(sessionID)
	s.dirty[key] = true
}

func (s *Store) KeyByRoot(root string) (repoid.ID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if p.Root == root {
			return repoid.ID{Key: p.Key, Identity: p.Identity, Root: p.Root, Label: p.Label}, true
		}
	}
	return repoid.ID{}, false
}

func (s *Store) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.projects[key] != nil
}

// Merge moves a path-keyed project into its repo identity (a folder that
// became a git repo). Sessions present in both are kept from the target.
func (s *Store) Merge(fromKey string, to repoid.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.projects[fromKey]
	if from == nil || fromKey == to.Key {
		return
	}
	dst := s.projects[to.Key]
	if dst == nil {
		dst = &Project{Key: to.Key, Identity: to.Identity, Root: to.Root, Label: to.Label}
		s.projects[to.Key] = dst
		dst.Plot = from.Plot
	}
	have := map[string]bool{}
	for _, ss := range dst.Sessions {
		have[ss.ID] = true
	}
	for _, ss := range from.Sessions {
		if !have[ss.ID] {
			dst.Sessions = append(dst.Sessions, ss)
		}
	}
	for f, t := range from.Files {
		if dst.Files == nil {
			dst.Files = map[string]FileTouch{}
		}
		d := dst.Files[f]
		d.Reads, d.Edits, d.Writes = d.Reads+t.Reads, d.Edits+t.Edits, d.Writes+t.Writes
		if t.LastAt > d.LastAt {
			d.LastAt = t.LastAt
		}
		dst.Files[f] = d
	}
	for u, o := range from.SeenTurns {
		if dst.SeenTurns == nil {
			dst.SeenTurns = map[string]string{}
		}
		dst.SeenTurns[u] = o
	}
	for path, c := range from.Cursors {
		if dst.Cursors == nil {
			dst.Cursors = map[string]Cursor{}
		}
		dst.Cursors[path] = c
		s.byPath[path] = to.Key
	}
	at := map[string]int{}
	for i, u := range dst.Unlocks {
		at[u.ID] = i
	}
	for _, u := range from.Unlocks { // earned is earned: keep the earlier unlock
		if i, ok := at[u.ID]; !ok {
			at[u.ID] = len(dst.Unlocks)
			dst.Unlocks = append(dst.Unlocks, u)
		} else if u.At < dst.Unlocks[i].At {
			dst.Unlocks[i] = u
		}
	}
	dst.recompute()
	s.dirty[to.Key] = true
	from.Cursors = nil // now owned by dst; forgetLocked must not unmap them
	s.forgetLocked(fromKey)
}

func (s *Store) Forget(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forgetLocked(key)
}

func (s *Store) forgetLocked(key string) error {
	p := s.projects[key]
	if p == nil {
		return nil
	}
	for path := range p.Cursors {
		delete(s.byPath, path)
	}
	delete(s.projects, key)
	delete(s.dirty, key)
	if s.readOnly {
		return nil
	}
	path := s.projectPath(key)
	for _, f := range []string{path, path + ".bak"} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *Store) Projects() []ProjectInfo {
	s.mu.Lock()
	out := make([]ProjectInfo, 0, len(s.projects))
	for _, p := range s.projects {
		info := ProjectInfo{Key: p.Key, Label: p.Label, Root: p.Root, FirstSeen: p.FirstSeen, LastSeen: p.LastSeen, Totals: p.Totals,
			ScannedAt: p.ScannedAt, ScanHead: p.ScanHead, Plot: p.Plot}
		if n := len(p.Snapshots); n > 0 {
			latest := p.Snapshots[n-1]
			info.Latest = &latest
		}
		out = append(out, info)
	}
	s.mu.Unlock()
	for i := range out {
		if _, err := os.Stat(out[i].Root); err != nil {
			out[i].Missing = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

// Project returns a deep copy.
func (s *Store) Project(key string) (Project, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return Project{}, false
	}
	b, _ := json.Marshal(p)
	var cp Project
	_ = json.Unmarshal(b, &cp)
	return cp, true
}

// Flush writes every dirty project atomically, taking the daily backup first.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readOnly {
		return nil
	}
	var first error
	if s.profileDirty {
		if err := writeJSON(filepath.Join(s.dir, "profile.json"), map[string]any{"unlocks": s.account}); err != nil {
			first = err
		} else {
			s.profileDirty = false
		}
	}
	for key := range s.dirty {
		path := s.projectPath(key)
		s.backupIfDue(path)
		if err := writeJSON(path, s.projects[key]); err != nil {
			s.lastErr = "could not save project history: " + err.Error()
			if first == nil {
				first = err
			}
			continue
		}
		delete(s.dirty, key)
	}
	return first
}

func (s *Store) backupIfDue(path string) {
	if fi, err := os.Stat(path + ".bak"); err == nil && s.now().Sub(fi.ModTime()) < 24*time.Hour {
		return
	}
	if b, err := os.ReadFile(path); err == nil {
		_ = writeFile(path+".bak", b)
	}
}

func (s *Store) Close() error {
	err := s.Flush()
	s.mu.Lock()
	if s.unlock != nil {
		s.unlock()
		s.unlock = nil
	}
	s.mu.Unlock()
	return err
}

func (s *Store) ReadOnly() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readOnly
}

func (s *Store) LastError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// DeleteAll removes the whole ledger directory (opt-out with delete).
func DeleteAll(dir string) error { return os.RemoveAll(dir) }

// Disable records an opt-out so the next launch does not resume recording.
// It needs no lock: only the consent file changes, and the data stays until
// DeleteAll.
func Disable(dir string) error {
	var c Consent
	path := filepath.Join(dir, consentFile)
	if err := readJSON(path, &c); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	c.Enabled = false
	return writeJSON(path, c)
}

// ApplyMetrics stores a scan of the project whose root is root, replacing the
// latest file metrics and upserting today's snapshot. False if root is unknown
// or excluded.
func (s *Store) ApplyMetrics(root string, r scan.Result) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isExcluded(root) {
		return false
	}
	for key, p := range s.projects {
		if p.Root != root {
			continue
		}
		p.Metrics, p.ScanHead, p.ScanPartial = r.Files, r.Head, r.Partial
		p.ScannedAt = s.now().UnixMilli()
		if n := len(p.Snapshots); n > 0 && p.Snapshots[n-1].Day == r.Snapshot.Day {
			p.Snapshots[n-1] = r.Snapshot
		} else {
			p.Snapshots = append(p.Snapshots, r.Snapshot)
		}
		s.dirty[key] = true
		return true
	}
	return false
}

// Metrics returns a project's latest file metrics and snapshot history.
func (s *Store) Metrics(key string) (map[string]scan.FileMetrics, []scan.Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return nil, nil, false
	}
	files := make(map[string]scan.FileMetrics, len(p.Metrics))
	for k, v := range p.Metrics {
		files[k] = v
	}
	return files, append([]scan.Snapshot(nil), p.Snapshots...), true
}

func (s *Store) RecordUnlocks(key string, us []Unlock) []Unlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return nil
	}
	have := map[string]bool{}
	for _, u := range p.Unlocks {
		have[u.ID] = true
	}
	var added []Unlock
	for _, u := range us {
		if !have[u.ID] {
			have[u.ID] = true
			p.Unlocks = append(p.Unlocks, u)
			added = append(added, u)
		}
	}
	if len(added) > 0 {
		s.dirty[key] = true
	}
	return added
}

func (s *Store) AccountUnlocks() []Unlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Unlock(nil), s.account...)
}

func (s *Store) RecordAccountUnlocks(us []Unlock) []Unlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	have := map[string]bool{}
	for _, u := range s.account {
		have[u.ID] = true
	}
	var added []Unlock
	for _, u := range us {
		if !have[u.ID] {
			have[u.ID] = true
			s.account = append(s.account, u)
			added = append(added, u)
		}
	}
	if len(added) > 0 {
		s.profileDirty = true
	}
	return added
}

// Sessions returns a deep copy of a project's session summaries.
func (s *Store) Sessions(key string) []SessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p.Sessions)
	var out []SessionSummary
	_ = json.Unmarshal(b, &out)
	return out
}

// Unlocks returns a copy of a project's achievement unlocks.
func (s *Store) Unlocks(key string) []Unlock {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.projects[key]; p != nil {
		return append([]Unlock(nil), p.Unlocks...)
	}
	return nil
}

// Progressable returns the parts of a project progression reads — sessions,
// snapshots, unlocks — as copies, without per-file metrics or cursors.
func (s *Store) Progressable(key string) (Project, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return Project{}, false
	}
	b, _ := json.Marshal(struct {
		Sessions  []SessionSummary
		Snapshots []scan.Snapshot
		Unlocks   []Unlock
	}{p.Sessions, p.Snapshots, p.Unlocks})
	var cp struct {
		Sessions  []SessionSummary
		Snapshots []scan.Snapshot
		Unlocks   []Unlock
	}
	_ = json.Unmarshal(b, &cp)
	return Project{Key: p.Key, Root: p.Root, Label: p.Label, Sessions: cp.Sessions, Snapshots: cp.Snapshots, Unlocks: cp.Unlocks}, true
}

// nextPlotLocked returns the smallest plot no project holds (1-based).
func (s *Store) nextPlotLocked() int {
	used := map[int]bool{}
	for _, p := range s.projects {
		used[p.Plot] = true
	}
	n := 1
	for used[n] {
		n++
	}
	return n
}

// assignPlotsLocked gives every project without a plot one, oldest first, so
// files written before plots existed get a stable, deterministic layout.
func (s *Store) assignPlotsLocked() {
	var todo []*Project
	for _, p := range s.projects {
		if p.Plot == 0 {
			todo = append(todo, p)
		}
	}
	sort.Slice(todo, func(i, j int) bool {
		if todo[i].FirstSeen != todo[j].FirstSeen {
			return todo[i].FirstSeen < todo[j].FirstSeen
		}
		return todo[i].Key < todo[j].Key
	})
	for _, p := range todo {
		p.Plot = s.nextPlotLocked()
		s.dirty[p.Key] = true
	}
}

// Touches returns a copy of the project's per-file touch counts (rel → touch).
func (s *Store) Touches(key string) map[string]FileTouch {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[key]
	if p == nil {
		return nil
	}
	out := make(map[string]FileTouch, len(p.Files))
	for k, v := range p.Files {
		out[k] = v
	}
	return out
}

// Roots maps every project key to its root, without touching the disk.
func (s *Store) Roots() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.projects))
	for k, p := range s.projects {
		out[k] = p.Root
	}
	return out
}
