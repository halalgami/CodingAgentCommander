package attach

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateRejectsEveryControlAndInvisibleCharacter(t *testing.T) {
	dir := t.TempDir()
	bad := []string{
		"a\nb", "a\rb", "a\tb", "a\x00b", "a\x1bb", "a\x7fb", "a\u0085b",
		"a\u2028b", "a\u2029b", // line / paragraph separator
		"a\u200bb", "a\u200cb", "a\u200db", "a\u2060b", "a\ufeffb", // zero-width
		"a\u202eb", "a\u2066b", "a\u061cb", "a\u200eb", // bidi controls
		"a\xffb",                       // not UTF-8
		"a\U000e0041b", "a\U000e007fb", // tag characters (invisible, smuggle text)
		"a\u00adb", "a\u2061b", "a\u2064b", "a\u180eb", // soft hyphen, invisible math, mongolian vowel separator
	}
	for _, name := range bad {
		_, err := Validate(filepath.Join(dir, name))
		if !errors.Is(err, ErrControlChar) {
			t.Errorf("Validate(%q) err = %v, want ErrControlChar", name, err)
		}
	}
}

func TestValidateRejectsRelativeAndEmpty(t *testing.T) {
	for _, p := range []string{"", "name.pdf", "./rel.png", "../x.png"} {
		if _, err := Validate(p); !errors.Is(err, ErrNotAbsolute) {
			t.Errorf("Validate(%q) err = %v, want ErrNotAbsolute", p, err)
		}
	}
}

// A newline-split drop fragment like "name.pdf" must be refused even when a
// file of that name happens to exist in the process's cwd: resolving it with
// filepath.Abs would attach a file the user never chose.
func TestValidateNeverResolvesARelativeName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "name.pdf"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := Validate("name.pdf"); !errors.Is(err, ErrNotAbsolute) {
		t.Errorf("relative name that exists in cwd: err = %v, want ErrNotAbsolute", err)
	}
}

func TestValidateAcceptsAwkwardLegalNames(t *testing.T) {
	dir := t.TempDir()
	names := []string{"has space.pdf", "it's.txt", "$HOME.md", "\u00fcn\u00efc\u00f8d\u00e9.png", "-leading-dash.txt", "(paren) [br].jpg",
		"\u2764\ufe0f heart.png", "\u263a\ufe0e text.png", "\U0001f600 grin.png"} // VS16/VS15 are Mn, not Cf
	if runtime.GOOS != "windows" {
		names = append(names, `quote"s.png`) // '"' is illegal in Windows file names
	}
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := Validate(p)
		if err != nil {
			t.Errorf("Validate(%q) unexpected err %v", name, err)
		}
		if got != p {
			t.Errorf("Validate(%q) = %q, want %q", name, got, p)
		}
	}
}

func TestValidateMissingAndFolder(t *testing.T) {
	dir := t.TempDir()
	if _, err := Validate(filepath.Join(dir, "nope.pdf")); !errors.Is(err, ErrMissing) {
		t.Errorf("missing file err = %v, want ErrMissing", err)
	}
	got, err := Validate(dir)
	if err != nil || got != dir {
		t.Errorf("folder: got %q, %v; want %q, nil", got, err, dir)
	}
}

func TestKindOf(t *testing.T) {
	cases := map[string]string{"a.PNG": "image", "a.jpeg": "image", "a.webp": "image", "a.gif": "image", "a.pdf": "file", "a": "file"}
	for p, want := range cases {
		if got := KindOf(p, false); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", p, got, want)
		}
	}
	if got := KindOf("dir.png", true); got != "folder" {
		t.Errorf("a directory named like an image must be a folder, got %q", got)
	}
}

func TestPromptTextRelativeInsideCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX escaping rules")
	}
	cases := []struct{ abs, cwd, want string }{
		{"/p/proj/docs/spec.pdf", "/p/proj", "./docs/spec.pdf"},
		{"/p/proj/a.png", "/p/proj", "./a.png"},
		{"/p/other/a.png", "/p/proj", "/p/other/a.png"},
		{"/p/proj2/a.png", "/p/proj", "/p/proj2/a.png"},
		{"/p/..x/a.png", "/p", "./..x/a.png"},
		{"/p/proj", "/p/proj", "/p/proj"},
		{"/x/a.png", "", "/x/a.png"},
	}
	for _, c := range cases {
		if got := PromptText(c.abs, c.cwd); got != c.want {
			t.Errorf("PromptText(%q, %q) = %q, want %q", c.abs, c.cwd, got, c.want)
		}
	}
}

func TestPromptTextEscapesLikeATerminalDrop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX escaping rules")
	}
	cases := map[string]string{
		"/x/My File (1).png":         `/x/My\ File\ \(1\).png`,
		`/x/quote"s.png`:             `/x/quote\"s.png`,
		"/x/it's.txt":                `/x/it\'s.txt`,
		"/x/$HOME.md":                `/x/\$HOME.md`,
		"/x/\u00fcn\u00ef.png":       "/x/\u00fcn\u00ef.png",
		"/x/-dash.txt":               "/x/-dash.txt",
		"/x/a@b+c=d,e%f:g.txt":       "/x/a@b+c=d,e%f:g.txt",
		"/x/at 10.00.00\u202fAM.png": "/x/at\\ 10.00.00\\\u202fAM.png",
	}
	for in, want := range cases {
		if got := PromptText(in, ""); got != want {
			t.Errorf("PromptText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPromptTextWindowsQuotesSpaces(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows quoting rules")
	}
	if got := PromptText(`C:\x\my file.png`, ""); got != `"C:\x\my file.png"` {
		t.Errorf("got %q", got)
	}
	if got := PromptText(`C:\x\a.png`, ""); got != `C:\x\a.png` {
		t.Errorf("got %q", got)
	}
}

func TestDisplayNameNeverCarriesRefusedCharacters(t *testing.T) {
	cases := map[string]string{
		"/x/evil\nname.pdf":     "evil?name.pdf",
		"/x/evil\u2028name.pdf": "evil?name.pdf",
		"/x/gpj.\u202eexe":      "gpj.?exe",
		"":                      "(no name)",
	}
	for in, want := range cases {
		if got := DisplayName(in); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}
