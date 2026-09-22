package delegate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const okResult = `{"status":"ok","answer":"it is here","citations":[{"file":"a.go","line":2,"quote":"package main"}]}`

func TestParseResultOK(t *testing.T) {
	r, err := ParseResult(okResult)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusOK || r.Answer != "it is here" || len(r.Citations) != 1 {
		t.Fatalf("got %+v", r)
	}
}

func TestParseResultRejectsUnknownStatus(t *testing.T) {
	_, err := ParseResult(`{"status":"maybe","answer":"x","citations":[]}`)
	if err == nil {
		t.Fatal("want an error for a status outside the enum")
	}
}

func TestParseResultAcceptsMissingCaveats(t *testing.T) {
	// Only status, answer and citations are required. Every extra mandatory
	// field is a turn spent on bookkeeping by a model with fewer to spare.
	if _, err := ParseResult(okResult); err != nil {
		t.Fatalf("caveats must be optional: %v", err)
	}
}

func TestParseResultStripsCodeFence(t *testing.T) {
	fenced := "```json\n" + okResult + "\n```"
	if _, err := ParseResult(fenced); err != nil {
		t.Fatalf("a fenced payload must parse: %v", err)
	}
}

func TestParseResultIgnoresWorkerSuppliedMetrics(t *testing.T) {
	// A model cannot know its own token count; asking invites a plausible
	// fabrication. Run overwrites these, so parsing must not trust them.
	r, err := ParseResult(`{"status":"ok","answer":"x","citations":[],"tokens_in":999999,"model":"gpt-9"}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.TokensIn != 0 || r.Model != "" {
		t.Errorf("worker-supplied metrics must be discarded, got tokens_in=%d model=%q", r.TokensIn, r.Model)
	}
}

func TestSchemaJSONRequiresExactlyThree(t *testing.T) {
	// Decode rather than substring-match, so the assertion cannot pass
	// vacuously: every extra mandatory field is a turn a weaker model spends
	// on bookkeeping instead of work.
	var schema struct {
		Required   []string       `json:"required"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(SchemaJSON(), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	got := strings.Join(schema.Required, ",")
	if got != "status,answer,citations" {
		t.Errorf("required = %q, want exactly status,answer,citations", got)
	}
	if _, ok := schema.Properties["caveats"]; !ok {
		t.Error("caveats must be offered as an optional property")
	}
}

func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestVerifyCitationsMatch(t *testing.T) {
	root := writeFixture(t)
	got := VerifyCitations(root, []Citation{{File: "a.go", Line: 1, Quote: "package main"}})
	if !got[0].Verified {
		t.Error("an exact quote at the named line must verify")
	}
}

func TestVerifyCitationsWhitespaceTolerant(t *testing.T) {
	root := writeFixture(t)
	got := VerifyCitations(root, []Citation{{File: "a.go", Line: 3, Quote: "  func main() {}  "}})
	if !got[0].Verified {
		t.Error("leading/trailing whitespace must not defeat verification")
	}
}

func TestVerifyCitationsMismatchPassesThroughFlagged(t *testing.T) {
	root := writeFixture(t)
	got := VerifyCitations(root, []Citation{{File: "a.go", Line: 1, Quote: "package elsewhere"}})
	if len(got) != 1 {
		t.Fatalf("a mismatching citation must be kept, got %d", len(got))
	}
	if got[0].Verified {
		t.Error("a mismatching quote must be marked unverified")
	}
	if got[0].Quote != "package elsewhere" {
		t.Error("the original quote must survive so the reader can judge it")
	}
}

// A symlinked file inside root pointing outside root is refused by os.Lstat
// on the final path element. Pointing the link outside root alone does not
// prove this guard works: the containment check on the resolved path (see
// TestVerifyCitationsRefusesASymlinkedParent) would also catch it, so a
// mutation that swaps Lstat for Stat would still pass an outside-only test.
// The inside-pointing case below exercises Lstat on its own: EvalSymlinks
// would happily resolve it to a legitimate in-root file, so only Lstat
// refuses it.
func TestVerifyCitationsRefusesASymlinkedFile(t *testing.T) {
	root := writeFixture(t)
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("TOP SECRET LINE\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Points outside root.
	outsideLink := filepath.Join(root, "filelink.txt")
	if err := os.Symlink(outside, outsideLink); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	got := VerifyCitations(root, []Citation{{File: "filelink.txt", Line: 1, Quote: "TOP SECRET LINE"}})
	if got[0].Verified {
		t.Error("a symlinked file pointing outside root must not verify")
	}

	// Points inside root, at a real file this project owns. Only Lstat
	// catches this one: the resolved target is legitimately inside root, so
	// the containment check alone would pass it.
	insideLink := filepath.Join(root, "insidelink.go")
	if err := os.Symlink(filepath.Join(root, "a.go"), insideLink); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	got = VerifyCitations(root, []Citation{{File: "insidelink.go", Line: 1, Quote: "package main"}})
	if got[0].Verified {
		t.Error("a symlinked file pointing inside root must still be refused as a symlink")
	}
}

// A symlinked DIRECTORY inside root pointing outside root is not caught by
// os.Lstat, which only inspects the final path element ("secret.txt", a
// plain file at the resolved location) — it is caught by the containment
// check running again on the path AFTER filepath.EvalSymlinks resolves the
// symlinked parent.
func TestVerifyCitationsRefusesASymlinkedParent(t *testing.T) {
	root := writeFixture(t)
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("TOP SECRET LINE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "link")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	got := VerifyCitations(root, []Citation{{File: "link/secret.txt", Line: 1, Quote: "TOP SECRET LINE"}})
	if got[0].Verified {
		t.Error("a symlinked parent directory pointing outside root must not verify")
	}
}

func TestVerifyCitationsRefusesEscapes(t *testing.T) {
	root := writeFixture(t)
	for _, c := range []Citation{
		{File: "/etc/passwd", Line: 1, Quote: "root"},
		{File: "../outside.go", Line: 1, Quote: "x"},
		{File: "missing.go", Line: 1, Quote: "x"},
		{File: "a.go", Line: 9999, Quote: "x"},
		{File: "a.go", Line: 0, Quote: "x"},
		{File: "a.go", Line: 1, Quote: ""},
	} {
		got := VerifyCitations(root, []Citation{c})
		if got[0].Verified {
			t.Errorf("%+v must not verify", c)
		}
	}
}

// Verified is documented as the only mechanical defence against fabricated
// provenance, and runlog counts it as the paraphrase tripwire — so a quote that
// CANNOT fail the check is worse than no check. The emptiness guard used to
// read the raw string while the comparison read the trimmed one, so " " passed
// the guard and then matched every line via Contains(line, "").
func TestVerifyRejectsQuotesThatCannotDiscriminate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", " ", "\t\n ", "e", "func"} {
		if verifyOne(root, Citation{File: "f.go", Line: 2, Quote: q}) {
			t.Errorf("quote %q verified: it matches too much to be evidence", q)
		}
	}
	// A quote with real content still verifies, on the line it names.
	if !verifyOne(root, Citation{File: "f.go", Line: 2, Quote: "func main() {}"}) {
		t.Error("a genuine quote was refused")
	}
	if verifyOne(root, Citation{File: "f.go", Line: 1, Quote: "func main() {}"}) {
		t.Error("a genuine quote verified against the WRONG line")
	}
}
