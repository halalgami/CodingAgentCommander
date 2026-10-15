package ingest

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/live"
	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

// jsonPath escapes a path for splicing into a JSON string literal: Windows
// paths carry backslashes, which would otherwise read as escapes.
func jsonPath(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }

// cwd is a folder that is not a git repo, so identity is path-based and no git runs matter.
func fixture(cwd string) []string {
	cwd = jsonPath(cwd)
	ts := func(s int) string { return time.Date(2026, 10, 9, 9, 0, s, 0, time.UTC).Format(time.RFC3339Nano) }
	return []string{
		`{"type":"ai-title","aiTitle":"Fix VAT rounding","sessionId":"s1"}`,
		`{"type":"user","timestamp":"` + ts(1) + `","sessionId":"s1","cwd":"` + cwd + `","message":{"role":"user","content":"go"}}`,
		`{"type":"assistant","timestamp":"` + ts(2) + `","sessionId":"s1","cwd":"` + cwd + `","message":{"model":"m","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"` + cwd + `/a.go","old_string":"x","new_string":"y"}}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		`{"type":"user","timestamp":"` + ts(3) + `","sessionId":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`{"type":"system","subtype":"turn_duration","durationMs":4000,"timestamp":"` + ts(4) + `","sessionId":"s1","uuid":"td1"}`,
	}
}

func setup(t *testing.T) (root, cwd string, st *store.Store, in *Ingester, tr *live.Tracker) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "projects")
	cwd = filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(cwd, 0o755)
	os.MkdirAll(filepath.Join(root, "enc"), 0o755)
	var err error
	st, err = store.Create(filepath.Join(t.TempDir(), "ledger"), store.Consent{StoreTitles: true}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tr = live.New(nil)
	in = New(root, tr, repoid.NewResolver())
	in.SetStore(st)
	return
}

func projectTotals(t *testing.T, st *store.Store) store.Counters {
	t.Helper()
	ps := st.Projects()
	if len(ps) != 1 {
		t.Fatalf("projects = %d", len(ps))
	}
	return ps[0].Totals
}

func TestChunkedAppendsEqualWholeFile(t *testing.T) {
	_, cwd, st, in, _ := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	whole := strings.Join(fixture(cwd), "\n") + "\n"

	f, _ := os.Create(path)
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < len(whole); {
		n := 1 + rng.Intn(40)
		if i+n > len(whole) {
			n = len(whole) - i
		}
		f.WriteString(whole[i : i+n])
		f.Sync()
		i += n
		if err := in.ReadFile(context.Background(), path, true); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	got := projectTotals(t, st)

	_, cwd2, st2, in2, _ := setup(t)
	path2 := filepath.Join(in2.Root, "enc", "s1.jsonl")
	os.WriteFile(path2, []byte(strings.Join(fixture(cwd2), "\n")+"\n"), 0o600)
	in2.ReadFile(context.Background(), path2, true)
	want := projectTotals(t, st2)

	if got.ActiveMs != want.ActiveMs || got.Turns != want.Turns || got.Tools["Edit"] != want.Tools["Edit"] || got.Tokens != want.Tokens {
		t.Fatalf("chunked %+v != whole %+v", got, want)
	}
	if got.ActiveMs != 4000 || got.Tools["Edit"] != 1 {
		t.Fatalf("unexpected totals %+v", got)
	}
}

func TestPartialLineCountedOnceWhenCompleted(t *testing.T) {
	_, cwd, st, in, _ := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	lines := fixture(cwd)
	last := lines[len(lines)-1]
	head := strings.Join(lines[:len(lines)-1], "\n") + "\n" + last[:20]
	os.WriteFile(path, []byte(head), 0o600)
	in.ReadFile(context.Background(), path, true)
	if projectTotals(t, st).ActiveMs != 0 {
		t.Fatal("half-written turn counted")
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(last[20:] + "\n")
	f.Close()
	in.ReadFile(context.Background(), path, true)
	in.ReadFile(context.Background(), path, true)
	if got := projectTotals(t, st).ActiveMs; got != 4000 {
		t.Fatalf("completed turn counted %d ms, want 4000 exactly once", got)
	}
}

func TestTruncatedFileRederives(t *testing.T) {
	_, cwd, st, in, _ := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	all := strings.Join(fixture(cwd), "\n") + "\n"
	os.WriteFile(path, []byte(all+all[:0]), 0o600)
	in.ReadFile(context.Background(), path, false)
	os.WriteFile(path, []byte(strings.Join(fixture(cwd)[:2], "\n")+"\n"), 0o600) // rewritten shorter
	in.ReadFile(context.Background(), path, false)
	if got := projectTotals(t, st); got.ActiveMs != 0 || got.Turns != 0 {
		t.Fatalf("rewritten file must be re-derived from zero: %+v", got)
	}
}

func TestDeletedTranscriptKeepsSummary(t *testing.T) {
	_, cwd, st, in, _ := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(path, []byte(strings.Join(fixture(cwd), "\n")+"\n"), 0o600)
	in.ReadFile(context.Background(), path, false)
	os.Remove(path)
	in.ReadFile(context.Background(), path, false)
	if projectTotals(t, st).ActiveMs != 4000 {
		t.Fatal("history lost when Claude Code deleted the transcript")
	}
	if _, ok := st.Cursor(path); ok {
		t.Fatal("cursor for deleted file kept")
	}
}

func TestParserVersionBumpRederivesWithoutDoubling(t *testing.T) {
	_, cwd, st, in, _ := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(path, []byte(strings.Join(fixture(cwd), "\n")+"\n"), 0o600)
	in.ReadFile(context.Background(), path, false)

	// Simulate a cursor written by an older parser.
	c, _ := st.Cursor(path)
	c.ParserVersion = 0
	id := repoid.NewResolver().Resolve(context.Background(), cwd)
	st.Apply(id, path, c, false, nil, 0)

	in.ReadFile(context.Background(), path, false)
	if got := projectTotals(t, st); got.ActiveMs != 4000 || got.Turns != 1 {
		t.Fatalf("re-derive after parser bump: %+v", got)
	}
}

func TestTrackerGetsLiveStateAndTitle(t *testing.T) {
	_, cwd, _, in, tr := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(path, []byte(strings.Join(fixture(cwd)[:3], "\n")+"\n"), 0o600)
	in.Watch(path)
	in.Tail(context.Background())
	s, ok := tr.Get("s1")
	if !ok || s.State != live.Tool || s.Target != "a.go" || s.Title != "Fix VAT rounding" || s.External {
		t.Fatalf("live = %+v", s)
	}
}

func TestBackfillStopsWhenStoreDetached(t *testing.T) {
	_, cwd, _, in, _ := setup(t)
	for i := 0; i < 50; i++ {
		p := filepath.Join(in.Root, "enc", "s"+string(rune('a'+i%26))+string(rune('a'+i/26))+".jsonl")
		os.WriteFile(p, []byte(strings.Join(fixture(cwd), "\n")+"\n"), 0o600)
	}
	seen := 0
	in.OnProgress = func(p Progress) {
		seen = p.Files
		if p.Files == 3 {
			in.SetStore(nil) // user turned history off
		}
	}
	err := in.Backfill(context.Background())
	if !errors.Is(err, ErrNoStore) {
		t.Fatalf("err = %v", err)
	}
	if seen > 4 {
		t.Fatalf("backfill kept going after opt-out: %d files", seen)
	}
}

func TestNoStoreReadsRecentOnlyForTracker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	os.MkdirAll(filepath.Join(root, "enc"), 0o755)
	tr := live.New(nil)
	in := New(root, tr, repoid.NewResolver())
	cwd := t.TempDir()
	old := filepath.Join(root, "enc", "old.jsonl")
	os.WriteFile(old, []byte(strings.Replace(strings.Join(fixture(cwd), "\n")+"\n", "s1", "old", -1)), 0o600)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(old, past, past)
	recent := filepath.Join(root, "enc", "s1.jsonl")
	// A just-appended transcript carries current timestamps; the tracker
	// expires external sessions whose newest event is older than 15 minutes.
	live := strings.ReplaceAll(strings.Join(fixture(cwd), "\n")+"\n", "2026-10-09T09:00:0", time.Now().UTC().Format("2006-01-02T15:04:0"))
	os.WriteFile(recent, []byte(live), 0o600)

	in.Sweep(context.Background())
	if _, ok := tr.Get("old"); ok {
		t.Fatal("old transcript read without a store")
	}
	if s, ok := tr.Get("s1"); !ok || !s.External {
		t.Fatalf("recent external session not tracked: %+v", s)
	}
}

func TestExcludedTranscriptNotRereadEverySweep(t *testing.T) {
	_, cwd, st, in, _ := setup(t)
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(path, []byte(strings.Join(fixture(cwd), "\n")+"\n"), 0o600)
	c := st.Consent()
	c.Excluded = []string{repoid.NewResolver().Resolve(context.Background(), cwd).Root}
	st.SetConsent(c)
	in.ReadFile(context.Background(), path, false)
	fi, _ := os.Stat(path)
	if in.storeStale(st, fileInfo{path, fi.Size(), fi.ModTime().UnixMilli()}) {
		t.Fatal("excluded transcript would be re-read in full on every sweep")
	}
}

func TestOnEditReportsLiveEditActivity(t *testing.T) {
	_, cwd, _, in, _ := setup(t)
	var got []string
	in.OnEdit = func(c string) { got = append(got, c) }
	path := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(path, []byte(strings.Join(fixture(cwd), "\n")+"\n"), 0o600)
	in.ReadFile(context.Background(), path, false) // store-only read: no live activity
	if len(got) != 0 {
		t.Fatal("backfill read reported live edits")
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"assistant","timestamp":"2026-10-09T09:00:09Z","sessionId":"s1","cwd":"` + jsonPath(cwd) + `","message":{"model":"m","content":[{"type":"tool_use","id":"t9","name":"Write","input":{"file_path":"` + jsonPath(cwd) + `/b.go","content":"x"}}]}}` + "\n")
	f.Close()
	in.ReadFile(context.Background(), path, true)
	if len(got) != 2 || got[0] != cwd {
		t.Fatalf("OnEdit calls %v; want the fixture Edit and the Write, with cwd", got)
	}
}

func TestGhostProjectMergesIntoParentRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, parent, st, in, _ := setup(t)
	ghost := filepath.Join(parent, "agent-1")
	os.MkdirAll(ghost, 0o755)
	p1 := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(p1, []byte(strings.Join(fixture(ghost), "\n")+"\n"), 0o600)
	in.ReadFile(context.Background(), p1, false) // recorded under the folder's own path

	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = parent
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	os.RemoveAll(ghost)
	in.resolver = repoid.NewResolver()

	p2 := filepath.Join(in.Root, "enc", "s2.jsonl")
	s2 := strings.ReplaceAll(strings.Join(fixture(ghost), "\n"), `"s1"`, `"s2"`)
	os.WriteFile(p2, []byte(strings.ReplaceAll(s2, "td1", "td2")+"\n"), 0o600)
	in.ReadFile(context.Background(), p2, false)

	repo := repoid.NewResolver().Resolve(context.Background(), parent)
	if ps := st.Projects(); len(ps) != 1 || ps[0].Key != repo.Key || len(st.Sessions(repo.Key)) != 2 {
		t.Fatalf("ghost not merged into parent repo %s: %+v", repo.Key, ps)
	}
}

func TestDeletedNestedRepoKeepsItsOwnProject(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, outer, st, in, _ := setup(t)
	inner := filepath.Join(outer, "inner")
	os.MkdirAll(inner, 0o755)
	for _, dir := range []string{outer, inner} {
		for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", dir}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
	}
	p1 := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(p1, []byte(strings.Join(fixture(inner), "\n")+"\n"), 0o600)
	in.ReadFile(context.Background(), p1, false)
	innerKey := repoid.NewResolver().Resolve(context.Background(), inner).Key

	os.RemoveAll(inner)
	in.resolver = repoid.NewResolver()
	p2 := filepath.Join(in.Root, "enc", "s2.jsonl")
	s2 := strings.ReplaceAll(strings.Join(fixture(inner), "\n"), `"s1"`, `"s2"`)
	os.WriteFile(p2, []byte(strings.ReplaceAll(s2, "td1", "td2")+"\n"), 0o600)
	in.ReadFile(context.Background(), p2, false)

	if ps := st.Projects(); len(ps) != 1 || ps[0].Key != innerKey || len(st.Sessions(innerKey)) != 2 {
		t.Fatalf("deleted nested repo lost its history to the outer repo: %+v", ps)
	}
}

func TestParserBumpFoldsExistingGhostWithoutDoubling(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, parent, st, in, _ := setup(t)
	ghost := filepath.Join(parent, "agent-1")
	os.MkdirAll(ghost, 0o755)
	p1 := filepath.Join(in.Root, "enc", "s1.jsonl")
	os.WriteFile(p1, []byte(strings.Join(fixture(ghost), "\n")+"\n"), 0o600)
	in.ReadFile(context.Background(), p1, false)
	g := st.Projects()[0]
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = parent
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	os.RemoveAll(ghost)
	in.resolver = repoid.NewResolver()

	c, _ := st.Cursor(p1) // as written by an older parser
	c.ParserVersion = 0
	st.Apply(repoid.ID{Key: g.Key, Identity: "path:" + g.Root, Root: g.Root, Label: g.Label}, p1, c, false, nil, 0)
	in.ReadFile(context.Background(), p1, false)

	repo := repoid.NewResolver().Resolve(context.Background(), parent)
	ps := st.Projects()
	if len(ps) != 1 || ps[0].Key != repo.Key || ps[0].Totals.ActiveMs != 4000 || ps[0].Totals.Turns != 1 {
		t.Fatalf("ghost after parser bump: %+v", ps)
	}
}
