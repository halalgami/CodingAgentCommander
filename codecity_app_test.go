package main

import (
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
)

func TestCodeViewLockedWithoutHistory(t *testing.T) {
	a := ledgerTestApp(t)
	if v := a.CodeView("k"); !v.Locked {
		t.Fatalf("%+v", v)
	}
	if err := a.ScanProject("k"); err == nil {
		t.Fatal("ScanProject without history must fail")
	}
}

func TestCodeViewFromTheStore(t *testing.T) {
	a := ledgerTestApp(t)
	st := attachTestStore(t, a)
	key := applyCommit(st, "/p1", "s1")
	v := a.CodeView(key)
	if v.Locked || v.Key != key || !v.NoMetrics {
		t.Fatalf("%+v", v)
	}
}

// Review Focus 1 + 2.
func TestTargetRelOnlyInsideTheProject(t *testing.T) {
	roots := map[string]string{"a": filepath.FromSlash("/code"), "b": filepath.FromSlash("/code/app")}
	s := live.Session{Cwd: filepath.FromSlash("/code/app/web"), TargetPath: filepath.FromSlash("/code/app/web/x.go"),
		Crew: []live.Agent{{ID: "1", TargetPath: filepath.FromSlash("/code/app/lib/y.go")}, {ID: "2", TargetPath: filepath.FromSlash("/Users/me/.zshrc")}}}
	got := relTargets(s, roots)
	if got.TargetRel != "web/x.go" || got.Crew[0].TargetRel != "lib/y.go" || got.Crew[1].TargetRel != "" {
		t.Fatalf("lead %q crew %q %q", got.TargetRel, got.Crew[0].TargetRel, got.Crew[1].TargetRel)
	}
	if s.Crew[0].TargetRel != "" {
		t.Fatal("relTargets must not mutate the input crew")
	}
	out := relTargets(live.Session{Cwd: filepath.FromSlash("/elsewhere"), TargetPath: filepath.FromSlash("/elsewhere/z.go")}, roots)
	if out.TargetRel != "" {
		t.Fatal("no project root → no rel")
	}
}

// Final review I3: a session in a nested project adds no live files to the parent's CodeView.
func TestLiveTargetsOnlyFromSessionsWhoseDeepestRootIsThisProject(t *testing.T) {
	roots := map[string]string{"a": filepath.FromSlash("/code"), "b": filepath.FromSlash("/code/app")}
	sess := []live.Session{
		{Cwd: filepath.FromSlash("/code/app"), TargetPath: filepath.FromSlash("/code/app/x.go")},
		{Cwd: filepath.FromSlash("/code/tools"), TargetPath: filepath.FromSlash("/code/tools/y.go"), Crew: []live.Agent{{ID: "1", TargetPath: filepath.FromSlash("/code/z.go")}}},
	}
	got := liveTargets(sess, roots, filepath.FromSlash("/code"))
	if len(got) != 2 || got[0] != "tools/y.go" || got[1] != "z.go" {
		t.Fatalf("parent targets %q", got)
	}
	if got := liveTargets(sess, roots, filepath.FromSlash("/code/app")); len(got) != 1 || got[0] != "x.go" {
		t.Fatalf("child targets %q", got)
	}
}
