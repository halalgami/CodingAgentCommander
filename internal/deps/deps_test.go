package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAtomicWritesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.exe")
	want := []byte("binary-ish")

	if err := writeAtomic(path, want, 0o755); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// The temp file must not survive: it sits in the same directory as the target,
// which is a directory this package puts on PATH. A leftover tmux.exe.tmp123 is
// harmless, but a pile of them accumulating on every launch is not.
func TestWriteAtomicLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeAtomic(filepath.Join(dir, "tool.exe"), []byte("x"), 0o755); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %d entries (%v), want only the target", len(entries), names)
	}
}

// Re-extracting over an existing binary is the upgrade path, so replacement must
// work rather than fail on an existing target.
func TestWriteAtomicReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(path, []byte("old-and-longer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("new"), 0o755); err != nil {
		t.Fatalf("writeAtomic over existing: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestUnderManagedDir(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"binary we extracted", filepath.Join(ManagedDir(), "bin", "psmux-3.3.8", "tmux.exe"), true},
		{"managed root itself", ManagedDir(), true},
		{"managed runtime tree", filepath.Join(ManagedDir(), "runtime", "pwsh-7.6.5", "pwsh.exe"), true},
		{"a system install", filepath.Join(string(filepath.Separator)+"usr", "local", "bin", "tmux"), false},
		{"sibling with shared prefix", ManagedDir() + "-other", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := underManagedDir(tc.path); got != tc.want {
				t.Errorf("underManagedDir(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestPrependPATHPutsDirFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/already/there")

	prependPATH(dir)

	entries := filepath.SplitList(os.Getenv("PATH"))
	if len(entries) == 0 || entries[0] != dir {
		t.Fatalf("PATH = %q, want %q first", os.Getenv("PATH"), dir)
	}
}

// Called on every install and every launch, so it must not grow PATH without
// bound.
func TestPrependPATHIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/already/there")

	prependPATH(dir)
	once := os.Getenv("PATH")
	prependPATH(dir)

	if got := os.Getenv("PATH"); got != once {
		t.Errorf("second call changed PATH:\n got %q\nwant %q", got, once)
	}
}

func TestPrependPATHIgnoresEmpty(t *testing.T) {
	t.Setenv("PATH", "/already/there")
	prependPATH("")
	if got := os.Getenv("PATH"); got != "/already/there" {
		t.Errorf("PATH = %q, want unchanged", got)
	}
}

// Status is what the UI renders, so every entry needs the fields it displays.
// It must also stay callable on a machine where nothing is installed — that is
// precisely when it gets called.
func TestStatusEntriesAreDescribed(t *testing.T) {
	st := Status()
	if len(st) == 0 {
		t.Fatal("Status returned no tools")
	}
	var sawClaude, sawTmux bool
	for _, tool := range st {
		if tool.Name == "" || tool.Label == "" {
			t.Errorf("tool %+v has no name or label", tool)
		}
		if !tool.Found && tool.Hint == "" {
			t.Errorf("%s is missing but offers no install hint", tool.Name)
		}
		if tool.Found && tool.Path == "" {
			t.Errorf("%s reports found with no path", tool.Name)
		}
		switch tool.Name {
		case "claude":
			sawClaude = true
		case "tmux":
			sawTmux = true
		}
	}
	if !sawTmux {
		t.Error("Status omits tmux, the session host")
	}
	if !sawClaude {
		t.Error("Status omits the claude CLI")
	}
}

// Missing must be a subset of Status limited to required-and-absent, since the
// UI turns it straight into a blocking prompt.
func TestMissingReportsOnlyRequiredAbsentTools(t *testing.T) {
	for _, tool := range Missing() {
		if !tool.Required {
			t.Errorf("%s is in Missing but is not required", tool.Name)
		}
		if tool.Found {
			t.Errorf("%s is in Missing but was found at %s", tool.Name, tool.Path)
		}
	}
}

// A hint that is not a runnable command is a dead end for the user reading it.
func TestHintsLookLikeCommands(t *testing.T) {
	for _, tool := range Status() {
		if tool.Hint == "" {
			continue
		}
		if !strings.ContainsAny(tool.Hint, " ") {
			t.Errorf("%s hint %q does not look like a command", tool.Name, tool.Hint)
		}
	}
}
