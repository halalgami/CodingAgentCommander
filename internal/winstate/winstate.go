// Package winstate remembers what Commander knows about each tmux window —
// which model it was launched on, and whether Remote Control is enabled — in a
// small JSON file beside the other generated state.
//
// This used to live in tmux user options (@commander_model, @commander_rc),
// which was durable and correct on upstream tmux. It is neither on psmux, the
// Windows shim: `set-option -w -t <window-id>` applies to every window rather
// than the targeted one, and `display-message -p -t <window-id>` ignores -t
// entirely and reports the active window. So every window read back the same
// value — whichever was written last — and a surviving window was reconciled to
// the wrong model, which SwapModel would then act on.
//
// A file is at least as durable as a tmux option: options live in the tmux
// server and survive app restarts, and so does this. It also survives a server
// restart, where the ids it holds become meaningless — see Prune.
package winstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Record is what is known about one window.
type Record struct {
	Model         string `json:"model,omitempty"`
	RemoteControl bool   `json:"remoteControl,omitempty"`
}

// Store is a window id -> Record map persisted to path. A Store with an empty
// path keeps everything in memory and never writes, which is what tests and a
// not-yet-started app use.
//
// Ids are unique within a tmux server, so they need no session qualifier; they
// are reused after a server restart, which is what Prune is for.
type Store struct {
	mu   sync.Mutex
	path string
	recs map[string]Record
}

// Open reads path, tolerating both a missing file (first run) and an unreadable
// one (hand-edited, truncated by a crash). A corrupt file is not worth failing
// launch over: the worst case is that surviving windows show the model recovered
// from their name, exactly as before this file existed.
func Open(path string) *Store {
	s := &Store{path: path, recs: map[string]Record{}}
	if path == "" {
		return s
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var recs map[string]Record
	if json.Unmarshal(body, &recs) != nil || recs == nil {
		return s
	}
	s.recs = recs
	return s
}

// Get returns the record for a window.
func (s *Store) Get(windowID string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[windowID]
	return r, ok
}

// Set replaces a window's record. A launch is authoritative: it overwrites
// whatever a previous window with this id left behind.
func (s *Store) Set(windowID string, r Record) error {
	if windowID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs[windowID] = r
	return s.save()
}

// Update mutates a window's record in place, creating it if absent.
func (s *Store) Update(windowID string, fn func(*Record)) error {
	if windowID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.recs[windowID]
	fn(&r)
	s.recs[windowID] = r
	return s.save()
}

// Delete forgets a window.
func (s *Store) Delete(windowID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.recs[windowID]; !ok {
		return nil
	}
	delete(s.recs, windowID)
	return s.save()
}

// Prune drops every record whose window is not in live, and reports how many
// went. Called with the reconciled window list, this is what keeps a restarted
// tmux server from handing a brand-new @1 the model of a long-dead one — and
// what stops the file growing without bound.
func (s *Store) Prune(live []string) (int, error) {
	keep := make(map[string]bool, len(live))
	for _, id := range live {
		keep[id] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for id := range s.recs {
		if !keep[id] {
			delete(s.recs, id)
			dropped++
		}
	}
	if dropped == 0 {
		return 0, nil
	}
	return dropped, s.save()
}

// save persists the map. Callers hold s.mu.
//
// Written to a temp file and renamed so a crash mid-write cannot leave a
// half-written file where Open expects JSON — the same rule config.Save follows.
func (s *Store) save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s.recs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
