package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

func TestMetricsNotPersistedWithoutConsent(t *testing.T) {
	a := ledgerTestApp(t)
	root := t.TempDir()
	initGit(t, root) // metrics exist only for git working trees
	r := scan.Result{Head: "h", Files: map[string]scan.FileMetrics{"x.go": {Flagged: true}}, Snapshot: scan.Snapshot{Day: "2026-10-11"}}
	a.ledger.onMetrics(root, r, nil)
	if _, err := os.Stat(a.ledgerDir()); !os.IsNotExist(err) {
		t.Fatal("metrics created the ledger without consent")
	}
	if !a.ledger.isHotspot(root, filepath.Join(root, "x.go")) {
		t.Fatal("in-memory hotspot not available without consent")
	}
}

func TestHotspotFlagFromSubfolderSession(t *testing.T) {
	a := ledgerTestApp(t)
	root := t.TempDir() // a git repo; a session in root/frontend must map to root
	os.MkdirAll(filepath.Join(root, "frontend"), 0o755)
	initGit(t, root)
	a.ledger.onMetrics(realPath(root), scan.Result{Files: map[string]scan.FileMetrics{"frontend/app.js": {Flagged: true}}}, nil)
	sub := filepath.Join(root, "frontend")
	if !a.ledger.isHotspot(sub, filepath.Join(sub, "app.js")) {
		t.Fatal("hotspot not found from a subfolder cwd")
	}
	a.sessions["@1"] = &sessionRec{Name: "p", Cwd: sub, Status: "active", ClaudeSessionID: "s1"}
	a.ledger.tracker.Apply([]parse.Event{{Kind: parse.ToolUse, SessionID: "s1", ToolID: "t", ToolName: "Edit", Target: "app.js", FilePath: filepath.Join(sub, "app.js")}}, false)
	if st := a.SessionStats("@1"); !st.Hotspot || st.Activity != string(live.Tool) {
		t.Fatalf("stats %+v", st)
	}
}

func initGit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "r"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
}

func realPath(p string) string { r, _ := filepath.EvalSymlinks(p); return r }

func TestHotspotInWorktreeSession(t *testing.T) {
	a := ledgerTestApp(t)
	main := t.TempDir()
	initGit(t, main)
	wt := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "worktree", "add", "-q", wt)
	cmd.Dir = main
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	// A scan of the worktree reports paths relative to the worktree itself.
	a.ledger.onMetrics(realPath(wt), scan.Result{Files: map[string]scan.FileMetrics{"app.js": {Flagged: true}}}, nil)
	if !a.ledger.isHotspot(wt, filepath.Join(wt, "app.js")) {
		t.Fatal("hotspot lost for a session working in a git worktree")
	}
	if a.ledger.isHotspot(main, filepath.Join(main, "app.js")) {
		t.Fatal("worktree hotspots leaked onto the main checkout")
	}
}
