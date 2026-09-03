package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// docWireKeys is LOCAL. The shared wireKeys lives in packgen_wire_test.go,
// which the export deletes by glob — calling it from a surviving file leaves
// the exported tree uncompilable, and the export's own `go test` gate fails.
func docWireKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(b)), "[") {
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			t.Fatalf("unmarshal array: %v", err)
		}
		if len(arr) == 0 {
			t.Fatal("empty payload: nothing to check a wire shape against")
		}
		b = arr[0]
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// An untagged exported field marshals under its Go name, and encoding/json
// UNmarshals case-insensitively — so the inbound direction works by luck while
// the outbound direction is silently empty. Asserting the FULL key set also
// catches a field renamed in Go without the frontend following.
func TestDocBindingsUseTheKeysTheViewerReads(t *testing.T) {
	root, _ := docFixture(t)
	a := NewApp()
	listing, err := a.ListProjectDocs(root)
	if err != nil {
		t.Fatal(err)
	}
	render, err := a.RenderProjectDoc(root, "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	linked, err := a.RenderProjectDoc(root, "haslink.md")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"DocListing", docWireKeys(t, listing), []string{"entries", "root", "truncated"}},
		{"DocEntry", docWireKeys(t, listing.Entries), []string{"modTime", "rel", "sinceStart", "size"}},
		{"DocRender", docWireKeys(t, render), []string{"css", "html", "kind", "lang", "links"}},
		{"DocLink", docWireKeys(t, linked.Links), []string{"href", "text"}},
	}
	for _, tc := range cases {
		if strings.Join(tc.got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s keys: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestRenderProjectDocRefusesWhatTheGuardRefuses(t *testing.T) {
	root, outside := docFixture(t)
	for _, rel := range []string{"", "../secret.md", filepath.Join(outside, "secret.md"), "gone.md", "logo.bin"} {
		if got, err := NewApp().RenderProjectDoc(root, rel); err == nil {
			t.Errorf("rendered %q: %+v", rel, got)
		}
	}
}

// OpenProjectDoc validates identically, and the opener is swappable so the
// refusals can be asserted without launching an application.
func TestOpenProjectDocRefusesTheSamePathsAndDoesNotSpawn(t *testing.T) {
	root, outside := docFixture(t)
	var opened []string
	restore := docOpener
	docOpener = func(path string) error { opened = append(opened, path); return nil }
	t.Cleanup(func() { docOpener = restore })

	for _, rel := range []string{
		"", "../secret.md", filepath.Join(outside, "secret.md"), "gone.md",
		"docs/../../" + filepath.Base(outside) + "/secret.md",
	} {
		if err := NewApp().OpenProjectDoc(root, rel); err == nil {
			t.Errorf("opened %q", rel)
		}
	}
	if len(opened) != 0 {
		t.Fatalf("spawned an opener for a refused path: %v", opened)
	}
}

// The escape hatch has to accept exactly what the renderer refuses (spec
// R2.3): binary files and oversized ones are the reason it exists.
func TestOpenProjectDocAcceptsWhatTheRendererDeclines(t *testing.T) {
	root, _ := docFixture(t)
	writeDoc(t, filepath.Join(root, "huge.md"), strings.Repeat("x", docsMaxBytes+1))
	var opened []string
	restore := docOpener
	docOpener = func(path string) error { opened = append(opened, path); return nil }
	t.Cleanup(func() { docOpener = restore })

	for _, rel := range []string{"logo.bin", "huge.md", "docs/deep.md"} {
		if err := NewApp().OpenProjectDoc(root, rel); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if len(opened) != 3 {
		t.Fatalf("opener got %v", opened)
	}
	for _, p := range opened {
		if !filepath.IsAbs(p) {
			t.Errorf("opener got a relative path: %q", p)
		}
	}
}

// A session's documents are its FOLDER's documents (spec R2.3/R3.3 — this app
// does not detect authorship), with recency relative to when the session
// started. Both facts come from the registry, so neither crosses the binding.
func TestListSessionDocsScopesToTheSessionsFolder(t *testing.T) {
	root, outside := docFixture(t)
	writeDoc(t, filepath.Join(outside, "elsewhere.md"), "# elsewhere")

	a := NewApp()
	a.mu.Lock()
	a.sessions["w1"] = &sessionRec{Cwd: root, LaunchedAt: time.Now().Add(-time.Hour)}
	a.mu.Unlock()

	got, err := a.ListSessionDocs("w1")
	if err != nil {
		t.Fatal(err)
	}
	names := rels(got)
	if !listed(names, "notes.md") {
		t.Errorf("notes.md missing from %v", names)
	}
	if listed(names, "elsewhere.md") {
		t.Error("listed a document from outside the session's folder")
	}
}

// SinceStart is the honest version of "from this session": mtime >= launch.
func TestListSessionDocsFlagsWhatChangedSinceLaunch(t *testing.T) {
	root, _ := docFixture(t)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "notes.md"), old, old); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	a.mu.Lock()
	a.sessions["w1"] = &sessionRec{Cwd: root, LaunchedAt: time.Now().Add(-time.Hour)}
	a.mu.Unlock()

	got, err := a.ListSessionDocs("w1")
	if err != nil {
		t.Fatal(err)
	}
	var since, before int
	for _, e := range got.Entries {
		if e.Rel == "notes.md" {
			if e.SinceStart {
				t.Error("notes.md predates the session but was flagged as changed since launch")
			}
			before++
			continue
		}
		if e.SinceStart {
			since++
		}
	}
	if before != 1 {
		t.Fatalf("the pre-dated document was not in the listing: %v", rels(got))
	}
	if since == 0 {
		t.Error("nothing was flagged SinceStart, so the flag is not being set at all")
	}
}

// ListProjectDocs has no session to be relative to, so it must not pretend.
func TestListProjectDocsNeverClaimsSinceStart(t *testing.T) {
	root, _ := docFixture(t)
	got, err := NewApp().ListProjectDocs(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got.Entries {
		if e.SinceStart {
			t.Errorf("%s was flagged sinceStart by a project-wide listing", e.Rel)
		}
	}
}

// The frontend can no longer disagree with Go about which root a listing
// belongs to (defect: an empty SessionStats.cwd made row clicks call openDoc
// with "", which Go then refused): DocListing.Root carries the value Go
// itself validated, resolved and absolute.
func TestListProjectDocsReturnsTheResolvedRoot(t *testing.T) {
	root, _ := docFixture(t)
	got, err := NewApp().ListProjectDocs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got.Root) {
		t.Errorf("root %q is not absolute", got.Root)
	}
	want, err := docRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != want {
		t.Errorf("root = %q, want %q", got.Root, want)
	}
}

// ListSessionDocs inherits its Root from the ListProjectDocs call it makes
// internally, so a session listing carries the session's own folder rather
// than whatever (possibly empty) cwd the frontend happened to have on hand.
func TestListSessionDocsRootIsTheSessionsFolder(t *testing.T) {
	root, _ := docFixture(t)
	a := NewApp()
	a.mu.Lock()
	a.sessions["w1"] = &sessionRec{Cwd: root, LaunchedAt: time.Now().Add(-time.Hour)}
	a.mu.Unlock()

	got, err := a.ListSessionDocs("w1")
	if err != nil {
		t.Fatal(err)
	}
	want, err := docRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != want {
		t.Errorf("root = %q, want %q", got.Root, want)
	}
}

func TestListSessionDocsErrorsOnAnUnknownSession(t *testing.T) {
	if _, err := NewApp().ListSessionDocs("nope"); err == nil {
		t.Fatal("an unknown window id was accepted")
	}
	if _, err := NewApp().ListSessionDocs(""); err == nil {
		t.Fatal("an empty window id was accepted")
	}
}
