package repoid

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "root")
	return dir
}

func TestRepoIdentitySurvivesMoveAndSubdir(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "frontend")
	os.MkdirAll(sub, 0o755)
	r := NewResolver()
	a := r.Resolve(context.Background(), sub)
	if !strings.HasPrefix(a.Identity, "rootcommit:") || a.Label != filepath.Base(repo) {
		t.Fatalf("subdir id = %+v", a)
	}
	moved := filepath.Join(t.TempDir(), "renamed")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}
	b := NewResolver().Resolve(context.Background(), moved)
	if b.Key != a.Key || b.Root == a.Root {
		t.Fatalf("moved repo: key %s vs %s, root %s vs %s", b.Key, a.Key, b.Root, a.Root)
	}
}

func TestWorktreeResolvesToMainRepo(t *testing.T) {
	repo := newRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, repo, "worktree", "add", "-q", wt)
	r := NewResolver()
	main, w := r.Resolve(context.Background(), repo), r.Resolve(context.Background(), wt)
	if main.Key != w.Key || main.Root != w.Root {
		t.Fatalf("worktree %+v vs main %+v", w, main)
	}
}

func TestNonRepoAndMissingFolderUsePath(t *testing.T) {
	plain := t.TempDir()
	id := NewResolver().Resolve(context.Background(), plain)
	if !strings.HasPrefix(id.Identity, "path:") || id.Key != id.PathKey {
		t.Fatalf("plain = %+v", id)
	}
	gone := filepath.Join(plain, "deleted-project")
	if g := NewResolver().Resolve(context.Background(), gone); !strings.HasPrefix(g.Identity, "path:") {
		t.Fatalf("missing = %+v", g)
	}
}

func TestMissingFolderInsideRepoWalksUpToRepo(t *testing.T) {
	repo := newRepo(t)
	gone := filepath.Join(repo, ".claude", "worktrees", "agent-1")
	os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	r := NewResolver()
	main, g := r.Resolve(context.Background(), repo), r.Resolve(context.Background(), gone)
	if g.Key != main.Key || g.Root != main.Root {
		t.Fatalf("deleted worktree %+v, want main repo %+v", g, main)
	}
	if want := clean(gone); g.Gone != want || g.PathKey != KeyOf("path:"+want) {
		t.Fatalf("Gone/PathKey = %q/%q, want the vanished path and its old path key", g.Gone, g.PathKey)
	}
}

func TestMissingFolderNeverWalksUpToHome(t *testing.T) {
	home := newRepo(t) // a dotfiles repo at $HOME must not swallow deleted projects
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads this on Windows
	gone := filepath.Join(home, "old-project")
	if g := NewResolver().Resolve(context.Background(), gone); !strings.HasPrefix(g.Identity, "path:") || g.Gone != "" {
		t.Fatalf("missing folder under a home repo = %+v", g)
	}
}

func TestCleanResolvesSymlinksOfMissingPath(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	want, _ := filepath.EvalSymlinks(real)
	if got := clean(filepath.Join(link, "gone", "deeper")); got != filepath.Join(want, "gone", "deeper") {
		t.Fatalf("clean = %s", got)
	}
}
