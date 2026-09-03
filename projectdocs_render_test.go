package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the .golden.html files")

func goldenCheck(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "docs", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v (run `go test -run TestRenderDoc -update` to create it, then READ it)", path, err)
	}
	if got != string(want) {
		t.Errorf("%s does not match. got:\n%s", path, got)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "docs", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The whole rendered surface, pinned. A golden file is what catches an
// upstream renderer changing its output from under us.
func TestRenderDocMarkdownGolden(t *testing.T) {
	got := renderDoc("kitchen-sink.md", fixture(t, "kitchen-sink.md"))
	if got.Kind != "markdown" {
		t.Errorf("kind = %q, want markdown", got.Kind)
	}
	goldenCheck(t, "kitchen-sink.golden.html", got.HTML)
}

// The assertions that matter most, stated separately from the golden so they
// survive a golden being regenerated carelessly.
func TestRenderDocDropsEverythingDangerous(t *testing.T) {
	html := renderDoc("kitchen-sink.md", fixture(t, "kitchen-sink.md")).HTML
	low := strings.ToLower(html)
	for _, forbidden := range []string{"<script", "onerror", "javascript:", "<img", "href=", " src=", " style=", " id="} {
		if strings.Contains(low, forbidden) {
			t.Errorf("rendered HTML contains %q:\n%s", forbidden, html)
		}
	}
}

// Structure survives sanitising — a policy that drops everything would pass
// the test above and be useless.
func TestRenderDocKeepsStructure(t *testing.T) {
	html := renderDoc("kitchen-sink.md", fixture(t, "kitchen-sink.md")).HTML
	for _, want := range []string{"<h1", "<h2", "<ul", "<ol", "<li", "<table", "<th", "<td",
		"<pre", "<code", "<blockquote", "<strong", "<em", "<del", "<hr", "class="} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML is missing %q:\n%s", want, html)
		}
	}
	// The anchor survives as an ELEMENT while the href does not: the reader
	// still sees that something was linked (and what its text was), the drawer
	// offers the target separately, and an href-less anchor navigates nothing.
	if !strings.Contains(html, "<a") {
		t.Error("the anchor element was stripped, so link text is indistinguishable from prose")
	}
	if !strings.Contains(html, "link") {
		t.Error("link text was dropped along with the href")
	}
}

// Links are extracted before sanitising and returned verbatim; the frontend
// classifies them (spec R2.4).
func TestRenderDocExtractsLinks(t *testing.T) {
	got := renderDoc("kitchen-sink.md", fixture(t, "kitchen-sink.md"))
	var hrefs []string
	for _, l := range got.Links {
		hrefs = append(hrefs, l.Href)
	}
	joined := strings.Join(hrefs, ",")
	for _, want := range []string{"./other.md", "https://example.com/path", "javascript:alert(1)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("links %v are missing %q", hrefs, want)
		}
	}
	for _, l := range got.Links {
		if l.Href == "./other.md" && l.Text != "link" {
			t.Errorf("link text = %q, want %q", l.Text, "link")
		}
	}
}

// Highlighting for anything that is not markdown, chosen by filename.
func TestRenderDocHighlightsSource(t *testing.T) {
	got := renderDoc("main.go", "package main\n\nfunc main() { println(\"hi\") }\n")
	if got.Kind != "text" {
		t.Errorf("kind = %q, want text", got.Kind)
	}
	if got.Lang == "" {
		t.Error("no lexer was chosen for a .go file")
	}
	if !strings.Contains(got.HTML, "class=") {
		t.Error("highlighted output carries no classes, so the stylesheet cannot reach it")
	}
	if strings.Contains(got.HTML, " style=") {
		t.Error("inline styles survived: the sanitizer policy forbids them, so highlighting must use classes")
	}
	if got.CSS == "" {
		t.Error("no stylesheet was returned for highlighted output")
	}
}

// An unknown extension still renders as plain text rather than erroring.
func TestRenderDocFallsBackToPlainText(t *testing.T) {
	got := renderDoc("notes.unknownext", "just some text\n<script>x</script>\n")
	if got.Kind != "text" {
		t.Errorf("kind = %q, want text", got.Kind)
	}
	if strings.Contains(strings.ToLower(got.HTML), "<script") {
		t.Error("a script tag survived the text path")
	}
	if !strings.Contains(got.HTML, "just some text") {
		t.Error("the text itself was lost")
	}
}

// The real corpus: this repository's own resume doc exercises tables, nested
// lists, long fences and inline HTML at realistic size. Skipped in the
// exported tree, where the export deletes it.
func TestRenderDocHandlesThisRepositorysOwnDocs(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("docs", "PROJECT_STATE.md"))
	if err != nil {
		t.Skip("PROJECT_STATE.md is absent: this is an exported tree")
	}
	got := renderDoc("docs/PROJECT_STATE.md", string(src))
	if len(got.HTML) < 10_000 {
		t.Errorf("rendered only %d bytes of HTML from a %d byte document", len(got.HTML), len(src))
	}
	if !strings.Contains(got.HTML, "<h2") {
		t.Error("no headings survived")
	}
	if strings.Contains(strings.ToLower(got.HTML), "href=") {
		t.Error("an href survived on the real corpus")
	}
}
