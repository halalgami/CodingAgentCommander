package safegit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestFsmonitorFromRepoConfigNeverRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := gitInit(t)
	marker := filepath.Join(t.TempDir(), "PWNED")
	script := filepath.Join(dir, "evil.sh")
	os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755)
	cfg := exec.Command("git", "config", "core.fsmonitor", script)
	cfg.Dir = dir
	if out, err := cfg.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	// Control: plain git status DOES run the repo's fsmonitor (proves the fixture works).
	ctl := exec.Command("git", "status")
	ctl.Dir = dir
	_ = ctl.Run()
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git does not invoke fsmonitor for status; fixture cannot prove anything")
	}
	os.Remove(marker)

	for _, sub := range [][]string{{"ls-files"}, {"rev-parse", "--show-toplevel"}, {"log", "--oneline", "-1"}} {
		if _, err := Run(context.Background(), dir, sub[0], sub[1:]...); err != nil {
			t.Fatalf("%v: %v", sub, err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("safegit %v executed the repo's core.fsmonitor", sub)
		}
	}
}

func TestDisallowedSubcommandRejected(t *testing.T) {
	for _, sub := range []string{"status", "fetch", "checkout", "config", "-c"} {
		if _, err := Run(context.Background(), t.TempDir(), sub); !errors.Is(err, ErrNotAllowed) {
			t.Fatalf("%s: err=%v, want ErrNotAllowed", sub, err)
		}
	}
}

func TestOutputCapIsEnforced(t *testing.T) {
	dir := gitInit(t)
	old := MaxOutput
	MaxOutput = 8
	defer func() { MaxOutput = old }()
	out, err := Run(context.Background(), dir, "rev-parse", "HEAD")
	if !errors.Is(err, ErrTruncated) || len(out) > 8 {
		t.Fatalf("out=%q (%d bytes) err=%v; want ≤8 bytes and ErrTruncated", out, len(out), err)
	}
}
