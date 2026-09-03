package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitRepo makes dir a git repository with one tracked doc, one untracked doc,
// one ignored doc, and a doc inside an ignored directory. Skips when git is
// absent — the fallback walk has its own tests, and a machine without git is
// not a reason to fail the suite.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	git(t, dir, "init")
	writeDoc(t, filepath.Join(dir, ".gitignore"), "ignored.md\nnode_modules/\n")
	writeDoc(t, filepath.Join(dir, "tracked.md"), "# tracked")
	writeDoc(t, filepath.Join(dir, "untracked.md"), "# untracked")
	writeDoc(t, filepath.Join(dir, "ignored.md"), "# ignored")
	writeDoc(t, filepath.Join(dir, "node_modules", "dep", "README.md"), "# dep")
	writeDoc(t, filepath.Join(dir, "notes.json"), `{"a":1}`)
	writeDoc(t, filepath.Join(dir, "main.go"), "package main")
	git(t, dir, "add", "tracked.md")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeDoc(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rels(l DocListing) []string {
	out := make([]string, 0, len(l.Entries))
	for _, e := range l.Entries {
		out = append(out, e.Rel)
	}
	return out
}

// listed is local on purpose: app.go's `contains` lives in a file the export
// replaces wholesale, and packgen's helpers live in files the export DELETES.
func listed(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Enumeration goes through git so .gitignore is honoured without this app
// shipping a gitignore parser, and so untracked-but-not-ignored files — the
// documents just written into a session — are still listed.
func TestListProjectDocsUsesGitAndRespectsIgnores(t *testing.T) {
	dir := gitRepo(t)
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := rels(got)
	for _, want := range []string{"tracked.md", "untracked.md", "notes.json"} {
		if !listed(names, want) {
			t.Errorf("%q is missing from %v", want, names)
		}
	}
	for _, bad := range []string{"ignored.md", "node_modules/dep/README.md"} {
		if listed(names, bad) {
			t.Errorf("%q was listed; it must not be", bad)
		}
	}
	// Listing scope is not reading scope (spec R2.3): source files are
	// READABLE by path but must not flood a twelve-row palette.
	if listed(names, "main.go") {
		t.Error("a source file was listed; the palette lists document-ish files only")
	}
	if got.Truncated {
		t.Error("a small repository reported Truncated")
	}
}

// A project root that is a SUBDIRECTORY of a repository is ordinary (any
// monorepo package) and must still get gitignore-aware listing. This is why
// nothing stats root/.git first: git resolves the repository itself, lists only
// what is under cwd, and prints paths relative to cwd.
func TestListProjectDocsUsesGitFromASubdirectoryRoot(t *testing.T) {
	repo := gitRepo(t)
	sub := filepath.Join(repo, "pkg")
	writeDoc(t, filepath.Join(sub, "inner.md"), "# inner")
	writeDoc(t, filepath.Join(sub, "ignored.md"), "# ignored by the root gitignore")
	got, err := NewApp().ListProjectDocs(sub)
	if err != nil {
		t.Fatal(err)
	}
	names := rels(got)
	if !listed(names, "inner.md") {
		t.Errorf("inner.md missing from %v", names)
	}
	if listed(names, "ignored.md") {
		t.Error("an ignored file was listed: the subdirectory root fell back to the walk")
	}
	if listed(names, "tracked.md") {
		t.Error("listed a file from ABOVE the chosen root")
	}
}

// git ls-files -c prints one line PER STAGE, so during a merge conflict the
// same file arrives three times — verified against a real conflicted repo. The
// symptom is three identical palette rows for one document.
func TestParseLsFilesDedupesAndCaps(t *testing.T) {
	out := []byte(strings.Join([]string{"a.md", "a.md", "a.md", "b.md", ""}, "\x00"))
	names, truncated := parseLsFiles(out)
	if len(names) != 2 || names[0] != "a.md" || names[1] != "b.md" {
		t.Fatalf("got %v, want [a.md b.md]", names)
	}
	if truncated {
		t.Error("two names reported truncation")
	}

	var many []string
	for i := 0; i < docsMaxEntries+5; i++ {
		many = append(many, fmt.Sprintf("d%05d.md", i))
	}
	names, truncated = parseLsFiles([]byte(strings.Join(many, "\x00")))
	if len(names) != docsMaxEntries || !truncated {
		t.Fatalf("cap: got %d names truncated=%v", len(names), truncated)
	}
}

// Paths cross the binding with forward slashes on every platform, because the
// frontend joins and compares them as strings.
func TestListProjectDocsUsesForwardSlashes(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "docs", "deep", "note.md"), "# note")
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !listed(rels(got), "docs/deep/note.md") {
		t.Fatalf("want docs/deep/note.md, got %v", rels(got))
	}
}

func TestListProjectDocsFallbackWalkSkipsHeavyDirs(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "README.md"), "# readme")
	writeDoc(t, filepath.Join(dir, "docs", "guide.markdown"), "# guide")
	for _, skipped := range []string{"node_modules", "vendor", "dist", "build", ".git"} {
		writeDoc(t, filepath.Join(dir, skipped, "inside.md"), "# no")
	}
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels(got)) != 2 {
		t.Fatalf("want exactly README.md + docs/guide.markdown, got %v", rels(got))
	}
}

// The git path and the walk path must answer the same question the same way.
// git pathspecs are case-SENSITIVE by default while the walk lowercases the
// extension, so without :(icase) the same README.MD is listed in a plain
// folder and invisible in a repository.
func TestListProjectDocsAgreesOnExtensionCase(t *testing.T) {
	repo := gitRepo(t)
	writeDoc(t, filepath.Join(repo, "SHOUTY.MD"), "# shouty")
	plain := t.TempDir()
	writeDoc(t, filepath.Join(plain, "SHOUTY.MD"), "# shouty")
	for _, dir := range []string{repo, plain} {
		got, err := NewApp().ListProjectDocs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !listed(rels(got), "SHOUTY.MD") {
			t.Errorf("%s: SHOUTY.MD missing from %v", dir, rels(got))
		}
	}
}

func TestListProjectDocsSortsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "old.md"), "# old")
	writeDoc(t, filepath.Join(dir, "new.md"), "# new")
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old.md"), old, old); err != nil {
		t.Fatal(err)
	}
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].Rel != "new.md" {
		t.Fatalf("want new.md first, got %v", rels(got))
	}
	if got.Entries[0].Size == 0 || got.Entries[0].ModTime == 0 {
		t.Errorf("entry carries no size/mtime: %+v", got.Entries[0])
	}
}

// A symlink is neither descended into nor listed. The DIRECTORY half is a
// property of filepath.WalkDir, not of our guard — verified by deleting the
// guard and watching it still pass — so the FILE half is what tests this code.
func TestListProjectDocsIgnoresSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	writeDoc(t, filepath.Join(outside, "secret.md"), "# secret")
	writeDoc(t, filepath.Join(dir, "own.md"), "# own")
	if err := os.Symlink(outside, filepath.Join(dir, "linkeddir")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "linkedfile.md")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rels(got) {
		if strings.HasPrefix(r, "linkeddir/") {
			t.Errorf("descended a symlinked directory: %v", rels(got))
		}
		if r == "linkedfile.md" {
			t.Errorf("listed a symlinked file: %v", rels(got))
		}
	}
	if !listed(rels(got), "own.md") {
		t.Errorf("dropped the real document: %v", rels(got))
	}
}

func TestListProjectDocsCapsAndReportsTruncation(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < docsMaxEntries+25; i++ {
		writeDoc(t, filepath.Join(dir, fmt.Sprintf("d%04d.md", i)), "# d")
	}
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) > docsMaxEntries {
		t.Errorf("returned %d entries, cap is %d", len(got.Entries), docsMaxEntries)
	}
	if !got.Truncated {
		t.Error("hit the cap without setting Truncated")
	}
}

// Exactly the cap is not truncation. The check runs after the append for this
// reason: on entry, any further visited path — a directory, a source file —
// flipped the flag on a listing that had lost nothing.
func TestListProjectDocsDoesNotClaimTruncationAtExactlyTheCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < docsMaxEntries; i++ {
		writeDoc(t, filepath.Join(dir, fmt.Sprintf("d%04d.md", i)), "# d")
	}
	writeDoc(t, filepath.Join(dir, "not-a-doc.bin"), "x")
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Truncated {
		t.Error("reported truncation for a listing that lost nothing")
	}
}

// The wall-clock budget is the other half of spec §2's guard, and it is a var
// precisely so this test can reach it.
func TestListProjectDocsStopsOnTheWallClockBudget(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 400; i++ {
		writeDoc(t, filepath.Join(dir, fmt.Sprintf("sub%03d", i), "d.md"), "# d")
	}
	restore := docsListBudget
	docsListBudget = time.Nanosecond
	t.Cleanup(func() { docsListBudget = restore })
	got, err := NewApp().ListProjectDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated {
		t.Error("blew the budget without setting Truncated")
	}
}

// A mistyped root must be distinguishable from a project with no documents.
func TestListProjectDocsErrorsOnABadRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file.md")
	writeDoc(t, file, "# a file")
	cases := []struct{ name, root string }{
		{"empty", ""},
		{"blank", "   "},
		{"missing", filepath.Join(t.TempDir(), "nope")},
		{"a file, not a directory", file},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewApp().ListProjectDocs(tc.root); err == nil {
				t.Fatalf("%q was accepted as a project root", tc.root)
			}
		})
	}
}
