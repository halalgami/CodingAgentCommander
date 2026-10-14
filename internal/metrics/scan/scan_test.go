package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/safegit"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) string {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, root, ".gitignore", "ignored.go\n")
	write(t, root, "hot.go", "package x\nfunc f(a bool) { if a && a { for {} } }\n")
	write(t, root, "cold.go", "package x\nvar v = 1\n")
	write(t, root, "a_test.go", "package x\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "one")
	for i := 0; i < 3; i++ { // churn on hot.go
		write(t, root, "hot.go", "package x\nfunc f(a bool) { if a && a { for {} } }\n// "+strings.Repeat("x", i+1)+"\n")
		git(t, root, "commit", "-qam", "churn")
	}
	git(t, root, "mv", "cold.go", "renamed.go")
	write(t, root, "renamed.go", "package x\nvar v = 1\nvar w = 2\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "rename")
	write(t, root, "new_untracked.go", "package x\n")
	write(t, root, "ignored.go", "package x\n")
	return root
}

func now() time.Time { return time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local) }

func TestRepoFilesChurnHotspots(t *testing.T) {
	root := repo(t)
	r, err := Repo(context.Background(), root, DefaultLimits, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Files["new_untracked.go"]; !ok {
		t.Error("untracked-not-ignored file missing")
	}
	if _, ok := r.Files["ignored.go"]; ok {
		t.Error("ignored file scanned")
	}
	if _, ok := r.Files["cold.go"]; ok {
		t.Error("renamed-away path still present")
	}
	hot := r.Files["hot.go"]
	if hot.Churn90 == 0 || !hot.Flagged || hot.Hotspot != 1 {
		t.Fatalf("hot.go %+v", hot)
	}
	if r.Files["renamed.go"].Churn90 == 0 {
		t.Error("rename churn not credited to the new path")
	}
	if !r.Files["a_test.go"].Test || r.Snapshot.Tests != 1 || r.Snapshot.Day != "2026-10-11" || r.Head == "" {
		t.Fatalf("snapshot %+v head %q", r.Snapshot, r.Head)
	}
	if r.Partial {
		t.Error("unexpected partial")
	}
}

func TestSymlinkEscapesAreSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := repo(t)
	outside := t.TempDir()
	write(t, outside, "secret.go", "package secret\nvar s = 1\n")
	os.Symlink(outside, filepath.Join(root, "link"))
	os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(root, "direct.go"))
	r, err := Repo(context.Background(), root, DefaultLimits, now)
	if err != nil {
		t.Fatal(err)
	}
	for p := range r.Files {
		if strings.Contains(p, "secret") || p == "direct.go" {
			t.Fatalf("read through a symlink: %s", p)
		}
	}
}

func TestCapsYieldPartial(t *testing.T) {
	root := repo(t)
	r, err := Repo(context.Background(), root, Limits{Wall: time.Minute, Files: 2, Bytes: 1 << 20, FileMax: 1 << 20}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Partial || !r.Snapshot.Partial || len(r.Files) > 2 {
		t.Fatalf("partial=%v files=%d", r.Partial, len(r.Files))
	}
}

func TestRenamedTo(t *testing.T) {
	for in, want := range map[string]string{
		"a/{old => new}/b.go": "a/new/b.go",
		"{a => b}/c.go":       "b/c.go",
		"old.go => new.go":    "new.go",
		"a/{ => sub}/x.go":    "a/sub/x.go",
		"plain.go":            "plain.go",
	} {
		if got := renamedTo(in); got != want {
			t.Errorf("%q → %q want %q", in, got, want)
		}
	}
}

func TestGitOutputTruncationMarksPartial(t *testing.T) {
	root := repo(t)
	old := safegit.MaxOutput
	safegit.MaxOutput = 40 // cuts ls-files and log output short
	defer func() { safegit.MaxOutput = old }()
	r, err := Repo(context.Background(), root, DefaultLimits, now)
	if err != nil {
		t.Fatalf("truncated git output must yield a partial result, not an error: %v", err)
	}
	if !r.Partial || !r.Snapshot.Partial {
		t.Fatal("truncated git output not reported as partial")
	}
}
