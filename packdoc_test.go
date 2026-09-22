package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pack-format document is named in the app's own UI, so it must live
// somewhere the public export keeps. docs/superpowers is deleted wholesale by
// scripts/export-public.sh, so a pointer into it ships a path the published
// repository does not contain.
//
// This test deliberately survives the export: the invariant matters more in
// the public tree than in this one.
const packDocPath = "docs/companion-pack-format.md"

func TestPackFormatDocIsPublished(t *testing.T) {
	if _, err := os.Stat(packDocPath); err != nil {
		t.Fatalf("%s: %v -- the UI points at this file", packDocPath, err)
	}
}

func TestNoShippedUIPointsIntoStrippedDocs(t *testing.T) {
	roots := []string{
		filepath.Join("frontend", "src", "lib", "companion"),
		filepath.Join("frontend", "src", "lib", "components"),
	}
	for _, root := range roots {
		// A root can legitimately be absent in an exported tree; a missing
		// directory is not drift.
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(p, ".svelte") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), "docs/superpowers/") {
				t.Errorf("%s names a docs/superpowers path, which the export deletes", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
