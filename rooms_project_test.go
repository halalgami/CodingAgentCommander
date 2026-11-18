package main

import (
	"os"
	"path/filepath"
	"testing"
)

// committedRepo: a git repo with one commit (so it has a root-commit identity).
func committedRepo(t *testing.T, dir string) string {
	t.Helper()
	gitRepo(t) // skips without git
	os.MkdirAll(dir, 0o755)
	git(t, dir, "init", "-q")
	writeDoc(t, filepath.Join(dir, "a.txt"), dir) // distinct root commit per repo
	git(t, dir, "add", "a.txt")
	git(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "first")
	return dir
}

func setUpRooms(t *testing.T) *App {
	t.Helper()
	a := roomsTestApp(t)
	if err := a.RoomsSetup(true, "Hussain"); err != nil {
		t.Fatal(err)
	}
	return a
}

func addRoomSession(a *App, id, cwd string) {
	a.mu.Lock()
	a.sessions[id] = &sessionRec{Name: id, Cwd: cwd}
	a.mu.Unlock()
}

func realpath(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProjectRoomsWorktreeMapsToMainRoom(t *testing.T) {
	a := setUpRooms(t)
	base := realpath(t, t.TempDir())
	main := committedRepo(t, filepath.Join(base, "proj"))
	wt := filepath.Join(base, "wt")
	git(t, main, "worktree", "add", "-q", wt)
	r, err := a.ShareProject(main)
	if err != nil {
		t.Fatal(err)
	}
	got := a.ProjectRooms([]string{wt})[wt]
	if got.RoomID != r.ID || got.Root != main {
		t.Fatalf("worktree maps to %+v, want room %s root %s", got, r.ID, main)
	}
}

func TestShareProjectBindsMainRootAndIsIdempotent(t *testing.T) {
	a := setUpRooms(t)
	base := realpath(t, t.TempDir())
	main := committedRepo(t, filepath.Join(base, "proj"))
	wt := filepath.Join(base, "wt")
	git(t, main, "worktree", "add", "-q", wt)
	r1, err := a.ShareProject(wt)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Folder != main || r1.Name != "proj" {
		t.Fatalf("shared %+v, want folder %s", r1, main)
	}
	r2, _ := a.ShareProject(main)
	if r2.ID != r1.ID || len(a.Rooms()) != 1 {
		t.Fatalf("second share made a new room: %s vs %s", r2.ID, r1.ID)
	}
}

func TestShareAfterCloseCreatesNewRoom(t *testing.T) {
	a := setUpRooms(t)
	main := committedRepo(t, filepath.Join(realpath(t, t.TempDir()), "proj"))
	r1, _ := a.ShareProject(main)
	if err := a.CloseRoom(r1.ID); err != nil {
		t.Fatal(err)
	}
	if got := a.ProjectRooms([]string{main})[main]; got.RoomID != "" {
		t.Fatalf("closed room still mapped: %+v", got)
	}
	r2, err := a.ShareProject(main)
	if err != nil || r2.ID == r1.ID {
		t.Fatalf("share after close: %+v %v", r2, err)
	}
}

func TestProjectRoomsNonGitFolder(t *testing.T) {
	a := setUpRooms(t)
	dir := realpath(t, t.TempDir())
	r, err := a.ShareProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.ProjectRooms([]string{dir})[dir]; got.RoomID != r.ID {
		t.Fatalf("non-git folder %+v", got)
	}
}

func TestSuggestFoldersSameIdentityOnly(t *testing.T) {
	a := setUpRooms(t)
	base := realpath(t, t.TempDir())
	main := committedRepo(t, filepath.Join(base, "proj"))
	clone := filepath.Join(base, "clone")
	git(t, base, "clone", "-q", main, clone)
	other := committedRepo(t, filepath.Join(base, "other"))
	r, _ := a.ShareProject(main)
	a.BindRoomFolder(r.ID, "") // pretend it arrived on this machine unbound
	addRoomSession(a, "w1", clone)
	addRoomSession(a, "w2", other)
	addRoomSession(a, "w3", main)
	got := a.SuggestFolders(r.ID)
	if len(got) != 2 {
		t.Fatalf("suggestions %+v", got)
	}
	for _, s := range got {
		if s.Path == other || s.Source != "session" {
			t.Fatalf("bad suggestion %+v", s)
		}
	}
}

func TestRevealFolderRefusesUnknownAndMissing(t *testing.T) {
	a := roomsTestApp(t)
	var opened []string
	old := revealDir
	revealDir = func(d string) error { opened = append(opened, d); return nil }
	t.Cleanup(func() { revealDir = old })
	if err := a.RevealFolder("nope"); err == nil {
		t.Fatal("unknown session accepted")
	}
	gone := filepath.Join(t.TempDir(), "gone")
	addRoomSession(a, "w1", gone)
	if err := a.RevealFolder("w1"); err == nil {
		t.Fatal("missing folder accepted")
	}
	dir := t.TempDir()
	addRoomSession(a, "w2", dir)
	if err := a.RevealFolder("w2"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != dir {
		t.Fatalf("opened %v", opened)
	}
}
