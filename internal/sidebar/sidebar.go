// Package sidebar persists the user's sidebar layout: named groups of
// sessions and the order of sessions inside each section.
//
// Sessions are keyed by tmux window id, like internal/winstate, and the layout
// is merged against the live window list on every reconcile. That merge is
// what keeps a restarted tmux server from handing a brand-new @1 the place of
// a long-dead one: dead ids are dropped before new ones are placed.
package sidebar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Ungrouped is the section key for sessions in no group.
const Ungrouped = "ungrouped"

// Group is one user-made section.
type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed,omitempty"`
	// Remember makes new sessions whose folder sits in Folders join this group.
	Remember bool     `json:"remember,omitempty"`
	Folders  []string `json:"folders,omitempty"`
}

// Layout is the whole sidebar: groups in display order, and per section key
// (Ungrouped or a group id) the window ids in display order.
type Layout struct {
	Groups []Group             `json:"groups"`
	Order  map[string][]string `json:"order"`
}

// Window is what Merge needs to know about a live tmux window.
type Window struct {
	ID  string
	Cwd string
}

// Store holds a Layout persisted to path. An empty path keeps it in memory.
type Store struct {
	mu   sync.Mutex
	path string
	l    Layout
}

// Open reads path. A missing file is a first run; an unreadable one is kept
// aside as path+".corrupt" and replaced by an empty layout, because losing a
// sidebar arrangement is never worth failing launch over.
func Open(path string) *Store {
	s := &Store{path: path, l: Normalize(Layout{})}
	if path == "" {
		return s
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var l Layout
	if json.Unmarshal(body, &l) != nil {
		_ = os.WriteFile(path+".corrupt", body, 0o600)
		return s
	}
	s.l = Normalize(l)
	return s
}

// Get returns a deep copy of the layout.
func (s *Store) Get() Layout {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.l)
}

// Set normalizes and stores l.
func (s *Store) Set(l Layout) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.l = Normalize(clone(l))
	return s.save()
}

// Merge drops ids not in live and places live ids the layout does not know:
// into the group remembering the deepest folder containing the window's cwd,
// else at the end of Ungrouped. It saves only when something changed.
func (s *Store) Merge(live []Window) (Layout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	alive := make(map[string]bool, len(live))
	for _, w := range live {
		alive[w.ID] = true
	}
	changed := false
	known := map[string]bool{}
	for key, list := range s.l.Order {
		kept := list[:0:0]
		for _, id := range list {
			if alive[id] {
				kept = append(kept, id)
				known[id] = true
			} else {
				changed = true
			}
		}
		s.l.Order[key] = kept
	}
	for _, w := range live {
		if known[w.ID] {
			continue
		}
		key := s.groupFor(w.Cwd)
		s.l.Order[key] = append(s.l.Order[key], w.ID)
		known[w.ID] = true
		changed = true
	}
	var err error
	if changed {
		err = s.save()
	}
	return clone(s.l), err
}

// groupFor picks the remembering group with the longest folder containing cwd.
func (s *Store) groupFor(cwd string) string {
	best, bestLen := Ungrouped, -1
	if cwd == "" {
		return best
	}
	for _, g := range s.l.Groups {
		if !g.Remember {
			continue
		}
		for _, f := range g.Folders {
			if within(cwd, f) && len(f) > bestLen {
				best, bestLen = g.ID, len(f)
			}
		}
	}
	return best
}

// within reports whether path is folder or below it, by path components.
func within(path, folder string) bool {
	if folder == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(folder), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Normalize makes a layout safe to render: every group has an order entry,
// sections with no group fold into Ungrouped, and an id appears once (its
// first appearance wins, Ungrouped first, then groups in display order).
func Normalize(l Layout) Layout {
	out := Layout{Groups: []Group{}, Order: map[string][]string{Ungrouped: {}}}
	seenGroup := map[string]bool{}
	for _, g := range l.Groups {
		if g.ID == "" || g.ID == Ungrouped || seenGroup[g.ID] {
			continue
		}
		seenGroup[g.ID] = true
		out.Groups = append(out.Groups, g)
		out.Order[g.ID] = []string{}
	}
	seen := map[string]bool{}
	add := func(key string, list []string) {
		for _, id := range list {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out.Order[key] = append(out.Order[key], id)
		}
	}
	add(Ungrouped, l.Order[Ungrouped])
	for _, g := range out.Groups {
		add(g.ID, l.Order[g.ID])
	}
	for key, list := range l.Order {
		if key != Ungrouped && !seenGroup[key] {
			add(Ungrouped, list)
		}
	}
	return out
}

func clone(l Layout) Layout {
	out := Layout{Groups: make([]Group, len(l.Groups)), Order: make(map[string][]string, len(l.Order))}
	for i, g := range l.Groups {
		g.Folders = append([]string(nil), g.Folders...)
		out.Groups[i] = g
	}
	for k, v := range l.Order {
		out.Order[k] = append([]string{}, v...)
	}
	return out
}

func (s *Store) save() error {
	if s.path == "" {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s.l, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".sidebar-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err = f.Write(body); err == nil {
		err = f.Chmod(0o600)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, s.path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
