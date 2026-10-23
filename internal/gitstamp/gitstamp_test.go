package gitstamp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestStampChangesWhenHeadMoves(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if _, ok := Stamp(dir); ok {
		t.Fatal("stamp before any commit: there is no reflog yet")
	}
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "one")
	a, ok := Stamp(dir)
	if !ok {
		t.Fatal("no stamp after a commit")
	}
	if b, _ := Stamp(dir); b != a {
		t.Fatal("stamp changed with no HEAD move")
	}
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "two")
	if b, _ := Stamp(dir); b == a {
		t.Fatal("stamp unchanged after a commit")
	}
}

func TestWorktreeHasItsOwnStamp(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "one")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, dir, "worktree", "add", "-q", wt)
	a, ok := Stamp(wt)
	if !ok {
		t.Fatal("no stamp for a linked worktree")
	}
	main, _ := Stamp(dir)
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "in worktree")
	if b, _ := Stamp(wt); b == a {
		t.Fatal("worktree stamp unchanged after a commit in it")
	}
	if b, _ := Stamp(dir); b != main {
		t.Fatal("a worktree commit moved the main tree's stamp")
	}
}

func TestNotARepo(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Stamp(dir); ok {
		t.Fatal("stamp for a plain folder")
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Stamp(dir); ok {
		t.Fatal("stamp from a malformed .git file")
	}
}
