package main

import (
	"context"
	"errors"
	"os"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
	"github.com/halalgami/CodingAgentCommander/internal/rooms"
	"github.com/halalgami/CodingAgentCommander/internal/safegit"
)

// Rooms anchored on projects (UX spec docs/superpowers/specs/2026-11-18-rooms-ux-design.md).
// A session's folder resolves to its project — identity = root commit, root =
// the main working tree, so worktrees and clones share a room. Resolution is
// cached per folder: cards ask on every poll and must never cost a git call.

var (
	projectResOnce sync.Once
	projectRes     *repoid.Resolver
)

func projects() *repoid.Resolver {
	projectResOnce.Do(func() { projectRes = repoid.NewResolver() })
	return projectRes
}

// ProjectRoom is what a session card needs to know about its project's room.
type ProjectRoom struct {
	Identity string `json:"identity"` // "rootcommit:…" or "" outside git
	Root     string `json:"root"`
	Label    string `json:"label"`
	RoomID   string `json:"roomId"` // the project's open room on this machine, or ""
}

// A folder outside git is asked again after this long: it may have become a
// repository (git init), and a repo's identity never changes (1.5a minor).
var (
	nonGitRecheck = 30 * time.Second
	nonGitMu      sync.Mutex
	nonGitAt      = map[string]time.Time{}
)

func resolveProject(cwd string) ProjectRoom {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	nonGitMu.Lock()
	if at, ok := nonGitAt[cwd]; ok && time.Since(at) >= nonGitRecheck {
		projects().Forget(cwd)
		delete(nonGitAt, cwd)
	}
	nonGitMu.Unlock()
	id := projects().Resolve(ctx, cwd)
	if !strings.HasPrefix(id.Identity, "rootcommit:") {
		nonGitMu.Lock()
		if _, ok := nonGitAt[cwd]; !ok {
			nonGitAt[cwd] = time.Now()
		}
		nonGitMu.Unlock()
	}
	p := ProjectRoom{Root: id.Root, Label: id.Label}
	if strings.HasPrefix(id.Identity, "rootcommit:") {
		p.Identity = id.Identity
	}
	return p
}

// roomFor: the open room of a project — by identity, or by folder outside git.
// Several open rooms for one project (made on two machines before they
// synced): the most recently active one wins, the same on every machine.
func roomFor(list []rooms.RoomSummary, p ProjectRoom) (rooms.RoomSummary, bool) {
	var best rooms.RoomSummary
	found := false
	for _, r := range list {
		if r.Closed {
			continue
		}
		if (p.Identity != "" && r.Project == p.Identity) || (p.Identity == "" && r.Project == "" && r.Folder == p.Root) {
			if !found || r.Last > best.Last || (r.Last == best.Last && r.ID < best.ID) {
				best, found = r, true
			}
		}
	}
	return best, found
}

// ProjectRooms maps each session folder to its project and open room.
func (a *App) ProjectRooms(cwds []string) map[string]ProjectRoom {
	out := map[string]ProjectRoom{}
	list := a.Rooms()
	for _, c := range cwds {
		if c == "" {
			continue
		}
		p := resolveProject(c)
		if r, ok := roomFor(list, p); ok {
			p.RoomID = r.ID
		}
		out[c] = p
	}
	return out
}

// ShareProject opens the project's room, creating it if there is none. A
// room that arrived here without a folder gets bound to this project.
func (a *App) ShareProject(cwd string) (rooms.RoomSummary, error) {
	s, err := a.rs()
	if err != nil {
		return rooms.RoomSummary{}, err
	}
	if strings.TrimSpace(cwd) == "" {
		return rooms.RoomSummary{}, errors.New("this session has no folder")
	}
	p := resolveProject(cwd)
	if r, ok := roomFor(s.Rooms(), p); ok {
		if r.Folder == "" {
			if err := s.BindFolder(r.ID, p.Root); err != nil {
				return r, err
			}
			r.Folder, r.NeedsFolder = p.Root, false
		}
		return r, nil
	}
	return s.CreateRoom(p.Root, p.Label, p.Identity)
}

// FolderSuggestion is a local folder that holds the same project as a room.
type FolderSuggestion struct {
	Path   string `json:"path"`
	Label  string `json:"label"`
	Source string `json:"source"` // session | history
}

// SuggestFolders ranks folders for a room that arrived without one: open
// sessions' projects first, then projects from history. Same identity only.
func (a *App) SuggestFolders(room string) []FolderSuggestion {
	out := []FolderSuggestion{}
	var want string
	for _, r := range a.Rooms() {
		if r.ID == room {
			want = r.Project
		}
	}
	if want == "" {
		return out
	}
	seen := map[string]bool{}
	add := func(path, label, src string) {
		if path != "" && !seen[path] {
			seen[path] = true
			out = append(out, FolderSuggestion{Path: path, Label: label, Source: src})
		}
	}
	a.mu.Lock()
	var cwds []string
	for _, r := range a.sessions {
		cwds = append(cwds, r.Cwd)
	}
	a.mu.Unlock()
	for _, c := range cwds {
		if p := resolveProject(c); p.Identity == want {
			add(p.Root, p.Label, "session")
		}
	}
	if a.ledger != nil {
		if st := a.ledger.currentStore(); st != nil {
			for _, p := range st.Projects() {
				if p.Key == repoid.KeyOf(want) && !p.Missing {
					add(p.Root, p.Label, "history")
				}
			}
		}
	}
	return out
}

// RevealFolder opens a session's folder in the file manager. It takes a
// session id, never a path: the page cannot point it anywhere else.
func (a *App) RevealFolder(windowID string) error {
	a.mu.Lock()
	r := a.sessions[windowID]
	a.mu.Unlock()
	if r == nil {
		return errors.New("unknown session")
	}
	if fi, err := os.Stat(r.Cwd); err != nil || !fi.IsDir() {
		return errors.New("that folder no longer exists")
	}
	return revealDir(r.Cwd)
}

// osFullName: the account's display name, to prefill "your name".
func osFullName() string {
	if u, err := user.Current(); err == nil {
		if n := strings.TrimSpace(u.Name); n != "" {
			return n
		}
		return u.Username
	}
	return ""
}

func (a *App) CloseRoom(room string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.CloseRoom(room)
}

func (a *App) LeaveRoom(room string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.LeaveRoom(room)
}

func (a *App) RemoveRoomLocal(room string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.RemoveLocal(room)
}

func (a *App) RestoreRoom(room string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.RestoreLocal(room)
}

func (a *App) HiddenRooms() []rooms.HiddenRoom {
	s, err := a.rs()
	if err != nil {
		return []rooms.HiddenRoom{}
	}
	return s.Hidden()
}

func (a *App) ResetRooms() error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.Reset()
}

func (a *App) CancelJoin() {
	if s, err := a.rs(); err == nil {
		s.CancelJoin()
	}
}

// Machines: my other machines and the network each is reached over (D10).
func (a *App) Machines() []rooms.PeerView {
	s, err := a.rs()
	if err != nil {
		return []rooms.PeerView{}
	}
	return s.Machines()
}

func (a *App) RenameRoom(room, name string) error {
	s, err := a.rs()
	if err != nil {
		return err
	}
	return s.RenameRoom(room, name)
}

// GitState is a session's branch and commit, for the handoff prefill.
type GitState struct {
	Branch string `json:"branch"`
	Sha    string `json:"sha"`
}

// GitHead reads a session's branch and HEAD. Session id only, never a path.
func (a *App) GitHead(windowID string) (GitState, error) {
	a.mu.Lock()
	r := a.sessions[windowID]
	a.mu.Unlock()
	if r == nil {
		return GitState{}, errors.New("unknown session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sha, err := safegit.Run(ctx, r.Cwd, "rev-parse", "HEAD")
	if err != nil {
		return GitState{}, errors.New("not a git repository")
	}
	branch, _ := safegit.Run(ctx, r.Cwd, "rev-parse", "--abbrev-ref", "HEAD")
	return GitState{Branch: strings.TrimSpace(branch), Sha: strings.TrimSpace(sha)}, nil
}

func (a *App) ledgerProjects() []store.ProjectInfo {
	if a.ledger == nil {
		return nil
	}
	st := a.ledger.currentStore()
	if st == nil {
		return nil
	}
	return st.Projects()
}

// recentProjects: Al Asturlab's projects, newest first, with their rooms.
func recentProjects(infos []store.ProjectInfo, list []rooms.RoomSummary, n int) []ProjectRoom {
	live := make([]store.ProjectInfo, 0, len(infos))
	for _, p := range infos {
		if !p.Missing && p.Root != "" {
			live = append(live, p)
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return live[i].LastSeen > live[j].LastSeen })
	out := []ProjectRoom{}
	for _, p := range live {
		if len(out) == n {
			break
		}
		out = append(out, projectRoomAt(p, list))
	}
	return out
}

func projectRoomAt(p store.ProjectInfo, list []rooms.RoomSummary) ProjectRoom {
	pr := resolveProject(p.Root)
	if pr.Label == "" {
		pr.Label = p.Label
	}
	if r, ok := roomFor(list, pr); ok {
		pr.RoomID = r.ID
	}
	return pr
}

func projectByKey(infos []store.ProjectInfo, list []rooms.RoomSummary, key string) ProjectRoom {
	for _, p := range infos {
		if p.Key == key && !p.Missing && p.Root != "" {
			return projectRoomAt(p, list)
		}
	}
	return ProjectRoom{}
}

// RecentProjects: Al Asturlab projects for the switcher's Share list.
func (a *App) RecentProjects() []ProjectRoom {
	return recentProjects(a.ledgerProjects(), a.Rooms(), 8)
}

// ProjectRoomByKey: an Al Asturlab project (ledger key) and its room.
func (a *App) ProjectRoomByKey(key string) ProjectRoom {
	return projectByKey(a.ledgerProjects(), a.Rooms(), key)
}
