package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestUserPath(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	got := UserPath()
	want := filepath.Join("/home/tester", ".config", "commander", "config.toml")
	if got != want {
		// NOT os.UserConfigDir(): on macOS that is ~/Library/Application
		// Support, and the app has always used ~/.config.
		t.Errorf("UserPath() = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, filepath.Join("commander", "config.toml")) {
		t.Errorf("unexpected suffix in %q", got)
	}
}
