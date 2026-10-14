//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The Lstat guard in serveAttachMedia leaves a window between check and open;
// O_NOFOLLOW closes it by refusing a symlink at open time.
func TestOpenNoFollowRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "t.png")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "l.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if f, err := openNoFollow(link); err == nil {
		f.Close()
		t.Error("opened a symlink")
	}
	f, err := openNoFollow(target)
	if err != nil {
		t.Fatalf("regular file: %v", err)
	}
	f.Close()
}
