package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestHeadCacheAsksGitOnlyWhenReflogMoves(t *testing.T) {
	dir := t.TempDir()
	initGit(t, dir)
	a := gitOut(t, dir, "rev-parse", "HEAD")
	gitOut(t, dir, "commit", "-q", "--allow-empty", "-m", "b")
	b := gitOut(t, dir, "rev-parse", "HEAD")
	hc := newHeadCache()
	if got, ok := hc.head(context.Background(), dir); !ok || got != b {
		t.Fatalf("head %q %v, want %s", got, ok, b)
	}
	// Move the branch behind git's back: no reflog entry, so the cache must
	// answer without running git — and still say b.
	ref := gitOut(t, dir, "symbolic-ref", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, ".git", filepath.FromSlash(ref)), []byte(a+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := hc.head(context.Background(), dir); got != b {
		t.Fatalf("head %q: asked git although the reflog did not move", got)
	}
	gitOut(t, dir, "commit", "-q", "--allow-empty", "-m", "c")
	c := gitOut(t, dir, "rev-parse", "HEAD")
	if got, _ := hc.head(context.Background(), dir); got != c {
		t.Fatalf("head %q after a commit, want %s", got, c)
	}
}

func TestHeadCacheWithoutReflogAsksGit(t *testing.T) {
	dir := t.TempDir()
	gitOut(t, dir, "init", "-q")
	gitOut(t, dir, "config", "core.logAllRefUpdates", "false")
	gitOut(t, dir, "commit", "-q", "--allow-empty", "-m", "a")
	hc := newHeadCache()
	if _, ok := hc.head(context.Background(), dir); !ok {
		t.Fatal("no head without a reflog")
	}
	gitOut(t, dir, "commit", "-q", "--allow-empty", "-m", "b")
	want := gitOut(t, dir, "rev-parse", "HEAD")
	if got, _ := hc.head(context.Background(), dir); got != want {
		t.Fatalf("head %q, want %s: a repo without a reflog must ask git each time", got, want)
	}
}
