//go:build windows

package deps

import (
	"archive/zip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The pin is the whole integrity story for a 106 MB download, so a malformed or
// missing one must fail here rather than at install time on a user's machine.
func TestPwshAssetIsPinnedForThisArch(t *testing.T) {
	name, sha, err := pwshAsset()
	if err != nil {
		t.Fatalf("pwshAsset: %v", err)
	}
	if !strings.HasPrefix(name, "PowerShell-"+PwshVersion+"-win-") || !strings.HasSuffix(name, ".zip") {
		t.Errorf("asset name %q does not match the pinned version %s", name, PwshVersion)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(sha) {
		t.Errorf("pinned hash %q is not a lowercase hex sha256", sha)
	}
}

// Every pinned hash must be a well-formed sha256, not just the one for the arch
// this test happens to run on — the arm64 entry is otherwise never exercised.
func TestAllPwshPinsAreWellFormed(t *testing.T) {
	hex := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for arch, sha := range pwshZipSHA256 {
		if !hex.MatchString(sha) {
			t.Errorf("pin for %s = %q, want a lowercase hex sha256", arch, sha)
		}
		if _, ok := pwshArch[arch]; !ok {
			t.Errorf("pin for %s has no asset-name mapping in pwshArch", arch)
		}
	}
}

func TestManagedPwshLivesUnderManagedDir(t *testing.T) {
	if !underManagedDir(ManagedPwsh()) {
		t.Errorf("ManagedPwsh() = %q, which is outside ManagedDir() = %q", ManagedPwsh(), ManagedDir())
	}
	if filepath.Base(ManagedPwsh()) != "pwsh.exe" {
		t.Errorf("ManagedPwsh() = %q, want it to end in pwsh.exe", ManagedPwsh())
	}
}

// The version belongs in the path: an upgrade must land in a new directory
// rather than try to overwrite files a running pwsh holds open.
func TestManagedPwshDirIsVersionScoped(t *testing.T) {
	if !strings.Contains(ManagedPwshDir(), PwshVersion) {
		t.Errorf("ManagedPwshDir() = %q, want it to contain the version %s", ManagedPwshDir(), PwshVersion)
	}
}

func TestUnzipToExtractsEntries(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.zip")
	writeZip(t, src, map[string]string{
		"pwsh.exe":           "fake",
		"Modules/Mod/x.psd1": "nested",
	})

	dst := filepath.Join(t.TempDir(), "out")
	if err := unzipTo(src, dst); err != nil {
		t.Fatalf("unzipTo: %v", err)
	}
	// Slash-separated archive names must land as real nested directories.
	want := []struct{ rel, content string }{
		{"pwsh.exe", "fake"},
		{filepath.Join("Modules", "Mod", "x.psd1"), "nested"},
	}
	for _, w := range want {
		got, err := os.ReadFile(filepath.Join(dst, w.rel))
		if err != nil {
			t.Errorf("read %s: %v", w.rel, err)
			continue
		}
		if string(got) != w.content {
			t.Errorf("%s = %q, want %q", w.rel, got, w.content)
		}
	}
}

// The archive is pinned and hash-verified, but honouring a "../" entry would
// write outside the destination — cheap to refuse, expensive to get wrong.
func TestUnzipToRejectsPathTraversal(t *testing.T) {
	src := filepath.Join(t.TempDir(), "evil.zip")
	writeZip(t, src, map[string]string{"../escaped.txt": "pwned"})

	base := t.TempDir()
	dst := filepath.Join(base, "out")
	err := unzipTo(src, dst)
	if err == nil {
		t.Fatal("unzipTo accepted an entry that escapes the destination")
	}
	if !strings.Contains(err.Error(), "escapes destination") {
		t.Errorf("error = %v, want it to name the escaping entry", err)
	}
	if _, err := os.Stat(filepath.Join(base, "escaped.txt")); err == nil {
		t.Error("the escaping entry was written outside the destination")
	}
}

// writeZip builds a zip at path from name -> content. Names are written verbatim
// so a test can express a hostile entry.
func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
