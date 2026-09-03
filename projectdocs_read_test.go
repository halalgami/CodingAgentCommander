package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docFixture is a project root with legitimate files, plus a file outside it
// that nothing here may ever reach.
func docFixture(t *testing.T) (root, outside string) {
	t.Helper()
	root = t.TempDir()
	outside = t.TempDir()
	writeDoc(t, filepath.Join(root, "notes.md"), "# notes\n\nbody\n")
	writeDoc(t, filepath.Join(root, "..hidden.md"), "# leading dots are a legal name\n")
	writeDoc(t, filepath.Join(root, "docs", "deep.md"), "# deep\n")
	writeDoc(t, filepath.Join(root, "main.go"), "package main\n")     // readable by path
	writeDoc(t, filepath.Join(root, "notes.unknownext"), "just text") // sniffed as text
	writeDoc(t, filepath.Join(root, "haslink.md"), "see [other](./other.md)\n")
	writeDoc(t, filepath.Join(outside, "secret.md"), "# secret\n")
	if err := os.WriteFile(filepath.Join(root, "logo.bin"), []byte{0x89, 0x50, 0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

// Every case asserts a REFUSAL. The frontend hands back a path the backend
// gave it, which is not a reason to trust it.
func TestReadDocSourceRefusesEscapes(t *testing.T) {
	root, outside := docFixture(t)
	cases := []struct{ name, rel string }{
		{"empty", ""},
		{"blank", "   "},
		{"a parent climb", "../secret.md"},
		{"a deep parent climb", "../../../../etc/passwd"},
		{"a climb in the middle", "docs/../../" + filepath.Base(outside) + "/secret.md"},
		{"a bare parent segment", ".."},
		{"an absolute path", filepath.Join(outside, "secret.md")},
		{"a unix absolute path", "/etc/hosts"},
		{"a missing file", "gone.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := readDocSource(root, tc.rel)
			if err == nil {
				t.Fatalf("read %q, got %d bytes", tc.rel, len(body))
			}
			if strings.Contains(body, "secret") {
				t.Fatalf("leaked content from outside the project: %q", body)
			}
		})
	}
}

func TestReadDocSourceRefusesADirectory(t *testing.T) {
	root, _ := docFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "adir.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readDocSource(root, "adir.md"); err == nil {
		t.Fatal("read a directory")
	}
}

// Legal filenames a lexical "starts with .." check refuses. These are listable,
// so refusing them here produces a palette row that cannot be opened — the
// guard must reject the SEGMENT "..", not the prefix.
func TestReadDocSourceAcceptsLegalNamesThatLookLikeClimbs(t *testing.T) {
	root, _ := docFixture(t)
	if _, err := readDocSource(root, "..hidden.md"); err != nil {
		t.Fatalf("refused a legal filename: %v", err)
	}
}

// Reading scope is wider than listing scope (spec R2.3): a source file, and a
// file with an extension nobody has heard of, both read fine if they are text.
func TestReadDocSourceReadsAnyTextFile(t *testing.T) {
	root, _ := docFixture(t)
	for _, rel := range []string{"notes.md", "docs/deep.md", "main.go", "notes.unknownext"} {
		if _, err := readDocSource(root, rel); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
}

// Binary is decided by BYTES, not by extension, and refused with a message
// that points at the escape hatch.
func TestReadDocSourceRefusesBinary(t *testing.T) {
	root, _ := docFixture(t)
	_, err := readDocSource(root, "logo.bin")
	if err == nil {
		t.Fatal("read a binary file")
	}
	if !strings.Contains(err.Error(), "externally") {
		t.Errorf("the refusal does not point at external open: %v", err)
	}
}

func TestLooksBinary(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"plain ascii", []byte("hello\nworld\n"), false},
		{"utf8 text", []byte("héllo — wörld\n"), false},
		{"empty", []byte{}, false},
		{"a NUL byte", []byte("he\x00llo"), true},
		{"invalid utf8", []byte{0xff, 0xfe, 0xfd}, true},
		{"utf8 cut mid-rune at the sniff boundary", append([]byte(strings.Repeat("a", 8191)), 0xc3), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksBinary(tc.in); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// os.Lstat is what refuses the final symlink. The link must point INSIDE the
// project for this to mean anything: one pointing outside is already refused
// by the containment check on the resolved path, so an outside-only test
// passes with Lstat mutated to Stat and proves nothing.
func TestReadDocSourceRefusesSymlinks(t *testing.T) {
	root, outside := docFixture(t)
	if err := os.Symlink(filepath.Join(root, "notes.md"), filepath.Join(root, "inside-link.md")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	if body, err := readDocSource(root, "inside-link.md"); err == nil {
		t.Errorf("read through a symlink pointing inside the project: %q", body)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "outside-link.md")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	if body, err := readDocSource(root, "outside-link.md"); err == nil {
		t.Errorf("read through a symlink pointing outside the project: %q", body)
	}
}

// Lstat only inspects the LAST element, so a symlinked parent directory is a
// separate escape needing the containment check on the resolved path.
func TestReadDocSourceRefusesASymlinkedParent(t *testing.T) {
	root, outside := docFixture(t)
	if err := os.Symlink(outside, filepath.Join(root, "linkeddir")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	if body, err := readDocSource(root, "linkeddir/secret.md"); err == nil {
		t.Fatalf("read through a symlinked directory: %q", body)
	}
}

func TestReadDocSourceRefusesAnOversizedFile(t *testing.T) {
	root, _ := docFixture(t)
	writeDoc(t, filepath.Join(root, "huge.md"), strings.Repeat("x", docsMaxBytes+1))
	_, err := readDocSource(root, "huge.md")
	if err == nil {
		t.Fatal("read a file past the size cap")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("the error does not say what happened: %v", err)
	}
}

// Exactly the cap is readable. The boundary matters because the read is capped
// with a LimitReader, where an off-by-one silently TRUNCATES a document
// instead of refusing it.
func TestReadDocSourceReadsExactlyTheCap(t *testing.T) {
	root, _ := docFixture(t)
	writeDoc(t, filepath.Join(root, "atcap.md"), strings.Repeat("y", docsMaxBytes))
	got, err := readDocSource(root, "atcap.md")
	if err != nil {
		t.Fatalf("refused a file exactly at the cap: %v", err)
	}
	if len(got) != docsMaxBytes {
		t.Fatalf("got %d bytes, want %d — the read was truncated", len(got), docsMaxBytes)
	}
}
