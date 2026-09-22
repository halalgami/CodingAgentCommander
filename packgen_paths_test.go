package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackGenSlug(t *testing.T) {
	cases := []struct {
		in, want, wantErr string
	}{
		{in: "Wafa", want: "wafa"},
		{in: "  My Companion  ", want: "my-companion"},
		{in: "late night v2", want: "late-night-v2"},
		{in: "a---b", want: "a-b"},
		{in: "--edges--", want: "edges"},
		{in: "MiXeD CaSe", want: "mixed-case"},

		// Traversal and absolute paths are not special-cased; the whitelist
		// leaves nothing of them to be dangerous with.
		{in: "../../etc/passwd", want: "etc-passwd"},
		{in: `C:\Windows\System32`, want: "c-windows-system32"},
		{in: "/etc/shadow", want: "etc-shadow"},
		{in: "a/b", want: "a-b"},
		{in: "a\x00b", want: "a-b"},
		{in: "a\nb\tc", want: "a-b-c"},
		{in: "pack.staging", want: "pack-staging"},
		{in: "...", wantErr: "no letters or digits"},
		{in: "", wantErr: "no letters or digits"},
		{in: "   ", wantErr: "no letters or digits"},
		{in: "日本語", wantErr: "no letters or digits"},

		// Windows device names, with and without an extension.
		{in: "CON", want: "con-pack"},
		{in: "nul", want: "nul-pack"},
		{in: "com1", want: "com1-pack"},
		{in: "con.png", want: "con-png"}, // not reserved once suffixed by the dot
		{in: "console", want: "console"}, // prefix only; not reserved
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := packGenSlug(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %q / %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The whitelist is the security property, so assert it directly over a range of
// hostile inputs rather than only through the table's expected values.
func TestPackGenSlugIsAlwaysASingleSafeSegment(t *testing.T) {
	inputs := []string{
		"../..", "..", ".", "a/../../b", `..\..\x`, "  ..  ", "x/../y",
		"~/.ssh/id_rsa", "%2e%2e%2f", "a:b", "a|b", "a*b", "a?b", `a"b`,
		"a<b>c", "‮exe.txt", "tab\there", "nul.txt", "trailing.",
		"trailing ", strings.Repeat("x", 500), strings.Repeat("héllo ", 40),
	}
	for _, in := range inputs {
		got, err := packGenSlug(in)
		if err != nil {
			continue // rejection is always an acceptable outcome
		}
		if got != filepath.Base(got) {
			t.Errorf("%q -> %q is not a single path segment", in, got)
		}
		if strings.ContainsAny(got, `/\:.`+"\x00") {
			t.Errorf("%q -> %q contains a forbidden character", in, got)
		}
		if got == "." || got == ".." || strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Errorf("%q -> %q is not a usable folder name", in, got)
		}
		if len(got) > packGenMaxSlug {
			t.Errorf("%q -> %q is %d bytes, over the %d cap", in, got, len(got), packGenMaxSlug)
		}
		for _, r := range got {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				t.Errorf("%q -> %q contains %q, outside the whitelist", in, got, r)
			}
		}
	}
}

// Promotion renames sibling folders inside the packs directory using dotted
// suffixes. That is only safe because no slug can contain a dot, so no
// suffixed name can ever be mistaken for — or collide with — a real pack.
func TestSuffixedNamesCannotCollideWithASlug(t *testing.T) {
	for _, suffix := range []string{packGenStagingSuffix, packGenOldSuffix} {
		if !strings.HasPrefix(suffix, ".") {
			t.Fatalf("suffix %q must start with a dot for the no-collision argument to hold", suffix)
		}
		// Try hard to produce a slug that looks like a suffixed folder.
		for _, attempt := range []string{"mine" + suffix, "mine." + suffix, "MINE" + strings.ToUpper(suffix)} {
			got, err := packGenSlug(attempt)
			if err != nil {
				continue
			}
			if strings.HasSuffix(got, suffix) {
				t.Errorf("slug %q from %q collides with the %q convention", got, attempt, suffix)
			}
		}
	}
}

func TestPackGenPathsForLivesUnderThePacksDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))

	paths, err := packGenPathsFor("My Companion")
	if err != nil {
		t.Fatal(err)
	}
	root := companionPacksDir()
	for _, p := range []string{paths.Live, paths.Staging, paths.Old} {
		if filepath.Dir(p) != root {
			t.Errorf("%s is not directly under %s", p, root)
		}
	}
	if paths.Slug != "my-companion" {
		t.Errorf("slug = %q", paths.Slug)
	}
	if paths.Staging != paths.Live+packGenStagingSuffix || paths.Old != paths.Live+packGenOldSuffix {
		t.Errorf("suffixes not applied to the live path: %+v", paths)
	}
}

// --- promotion -------------------------------------------------------------

func mkPack(t *testing.T, dir, marker string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMarker(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading marker in %s: %v", dir, err)
	}
	return string(b)
}

func promoteFixture(t *testing.T) packGenPaths {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	paths, err := packGenPathsFor("demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(companionPacksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestPromotePackFirstSave(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Staging, "new")

	if err := promotePack(paths); err != nil {
		t.Fatal(err)
	}
	if got := readMarker(t, paths.Live); got != "new" {
		t.Errorf("live marker = %q", got)
	}
	if packGenIsDir(paths.Staging) {
		t.Error("staging folder survived promotion")
	}
	if packGenIsDir(paths.Old) {
		t.Error("a backup was left behind when there was nothing to back up")
	}
}

func TestPromotePackReplacesExisting(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Live, "old")
	mkPack(t, paths.Staging, "new")

	if err := promotePack(paths); err != nil {
		t.Fatal(err)
	}
	if got := readMarker(t, paths.Live); got != "new" {
		t.Errorf("live marker = %q, want the newly generated pack", got)
	}
	if packGenIsDir(paths.Old) {
		t.Error("backup not cleaned up after a successful promotion")
	}
	if packGenIsDir(paths.Staging) {
		t.Error("staging folder survived promotion")
	}
}

// A crash between the two renames leaves the live folder missing and .old
// holding the user's ONLY copy. The library hides .old, regeneration refuses
// it, and the next promotion used to delete it — so the crash window silently
// destroyed a pack. An earlier version of this test asserted that deletion as
// correct behaviour, which documented the loss rather than preventing it.
func TestPromoteRecoversAPackStrandedByAnInterruptedSave(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Old, "the-users-only-copy")
	mkPack(t, paths.Staging, "new")

	if err := promotePack(paths); err != nil {
		t.Fatal(err)
	}
	// The new pack still lands...
	if got := readMarker(t, paths.Live); got != "new" {
		t.Errorf("live marker = %q, want the new pack", got)
	}
	// ...and the stranded one was put back first, so it was replaced through
	// the normal path rather than deleted out from under the user.
	if packGenIsDir(paths.Old) {
		t.Error("a backup was left behind after a successful promotion")
	}
}

// The recovery must also work when there is nothing to promote: a crash
// followed by the user simply reopening the app must not leave the pack
// invisible forever.
func TestPromoteRestoresEvenWithNothingStaged(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Old, "the-users-only-copy")

	err := promotePack(paths)
	if err == nil {
		t.Fatal("want an error: there is nothing staged")
	}
	if !packGenIsDir(paths.Live) {
		t.Fatal("the stranded pack was not restored")
	}
	if got := readMarker(t, paths.Live); got != "the-users-only-copy" {
		t.Errorf("restored marker = %q", got)
	}
}

func TestPromotePackWithNothingStaged(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Live, "old")

	err := promotePack(paths)
	if err == nil {
		t.Fatal("want an error when there is nothing staged")
	}
	if !strings.Contains(err.Error(), "nothing staged") {
		t.Errorf("unhelpful error: %v", err)
	}
	// The existing pack must be untouched by a failed save.
	if got := readMarker(t, paths.Live); got != "old" {
		t.Errorf("a failed promotion disturbed the live pack: %q", got)
	}
}

// The worst outcome promotion can produce is not a failed save — it is a save
// that fails AFTER moving the existing pack aside, leaving the user with no
// pack at all. The rename is driven through a seam so this exact sequence is
// what runs: backup succeeds, the move into place fails, restore must happen.
func TestPromotePackRestoresThePreviousPackWhenTheMoveFails(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Live, "old")
	mkPack(t, paths.Staging, "new")

	var calls int
	orig := promoteRename
	promoteRename = func(from, to string) error {
		calls++
		if calls == 2 { // the staging -> live move
			return errors.New("simulated failure")
		}
		return orig(from, to)
	}
	t.Cleanup(func() { promoteRename = orig })

	err := promotePack(paths)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "simulated failure") {
		t.Errorf("underlying cause not wrapped: %v", err)
	}
	if calls != 3 {
		t.Fatalf("want 3 renames (backup, failed move, restore), got %d", calls)
	}
	// The user must still have the pack they had before.
	if !packGenIsDir(paths.Live) {
		t.Fatal("the previous pack was not restored; the user is left with none")
	}
	if got := readMarker(t, paths.Live); got != "old" {
		t.Errorf("restored marker = %q, want the previous pack", got)
	}
	// And the staged art must survive so the save can be retried.
	if got := readMarker(t, paths.Staging); got != "new" {
		t.Errorf("staged marker = %q; a failed save destroyed generated art", got)
	}
}

// If the backup rename itself fails, nothing has moved yet, so promotion must
// abort without touching either folder.
func TestPromotePackLeavesEverythingAloneIfTheBackupFails(t *testing.T) {
	paths := promoteFixture(t)
	mkPack(t, paths.Live, "old")
	mkPack(t, paths.Staging, "new")

	orig := promoteRename
	promoteRename = func(from, to string) error { return errors.New("simulated failure") }
	t.Cleanup(func() { promoteRename = orig })

	if err := promotePack(paths); err == nil {
		t.Fatal("want an error")
	}
	if got := readMarker(t, paths.Live); got != "old" {
		t.Errorf("live marker = %q", got)
	}
	if got := readMarker(t, paths.Staging); got != "new" {
		t.Errorf("staged marker = %q", got)
	}
}
